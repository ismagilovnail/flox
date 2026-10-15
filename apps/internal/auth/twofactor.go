package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	totpengine "github.com/ismagilovnail/flox/apps/internal/totp"
)

// Second factor by email and SMS (docs/auth.md "Second factor: email and
// SMS"), beside the TOTP app: a user has one method (users.two_factor_
// method). Email and SMS send a 6-digit one-time code — kept only as its
// SHA-256, valid codeTTL, used once, codeAttempts wrong guesses. Sending is
// rate-limited per user (sendCooldown, sendHourlyCap): an SMS costs money,
// and a pumping attack would spend it.

// Two-factor methods (users.two_factor_method).
const (
	MethodTOTP  = "totp"
	MethodEmail = "email"
	MethodSMS   = "sms"
)

const (
	codeTTL       = 10 * time.Minute
	codeAttempts  = 3
	sendCooldown  = time.Minute
	sendHourlyCap = 5
	codeDigits    = 6
)

// SMSSender sends one text message (internal/smswebhook.Sender).
type SMSSender interface {
	SendSMS(ctx context.Context, to, text string) error
}

// SetSMSSender enables SMS codes; without one, SMS is not offered.
func (s *Service) SetSMSSender(sms SMSSender) { s.sms = sms }

// SMSAvailable reports whether SMS codes can be sent on this deployment.
func (s *Service) SMSAvailable() bool { return s.sms != nil }

// phonePattern is an E.164 number: +, country code, up to 15 digits.
var phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// NormalizePhone strips spaces, dashes and brackets from an E.164 number.
func NormalizePhone(raw string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '(', ')', '.':
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
}

// MaskEmail shows where a code went without the whole address:
// o***@flox.local.
func MaskEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return "***"
	}
	return local[:1] + "***@" + domain
}

// MaskPhone keeps the country prefix and the last 4 digits: +7******4567.
func MaskPhone(phone string) string {
	if len(phone) < 7 {
		return "***"
	}
	return phone[:2] + strings.Repeat("*", len(phone)-6) + phone[len(phone)-4:]
}

func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("auth: generating code: %w", err)
	}
	return fmt.Sprintf("%0*d", codeDigits, n.Int64()), nil
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

// TwoFactorStatus is a user's second factor as the security page shows it.
type TwoFactorStatus struct {
	Enabled      bool   `json:"enabled"`
	Method       string `json:"method,omitempty"`
	Phone        string `json:"phone,omitempty"` // masked
	SMSAvailable bool   `json:"smsAvailable"`
}

func (s *Service) TwoFactorStatus(ctx context.Context, userID string) (TwoFactorStatus, error) {
	st, err := s.repo.twoFactorState(ctx, userID)
	if err != nil {
		return TwoFactorStatus{}, err
	}
	out := TwoFactorStatus{Enabled: st.Method != "", Method: st.Method, SMSAvailable: s.SMSAvailable()}
	if st.Method == MethodSMS {
		out.Phone = MaskPhone(st.Phone)
	}
	return out, nil
}

// CodeSent is where a code went (masked) and which method sent it.
type CodeSent struct {
	Method string `json:"method"`
	SentTo string `json:"sentTo"`
}

// sendCode creates a code for userID and sends it by method to target,
// within the user's sending limits. pendingLoginID is set for a login
// code; backupHashes ride along with an enroll code.
func (s *Service) sendCode(ctx context.Context, userID, purpose, method, target, pendingLoginID string, backupHashes []string) (CodeSent, error) {
	if method == MethodSMS && s.sms == nil {
		return CodeSent{}, apierror.Unavailable("SMS codes are not available on this deployment")
	}
	last, hourly, err := s.repo.recentCodeSends(ctx, userID, purpose, time.Now().Add(-time.Hour))
	if err != nil {
		return CodeSent{}, err
	}
	if !last.IsZero() && time.Since(last) < sendCooldown {
		return CodeSent{}, apierror.TooManyRequests(fmt.Sprintf("a code was just sent — wait %d seconds before asking again", int((sendCooldown-time.Since(last)).Seconds())+1))
	}
	if hourly >= sendHourlyCap {
		return CodeSent{}, apierror.TooManyRequests("too many codes sent in the last hour — try again later")
	}
	code, err := newCode()
	if err != nil {
		return CodeSent{}, err
	}
	// The new code replaces the live ones of its purpose — only now that
	// it is going out: a refused resend leaves the sent code working.
	if err := s.repo.expireCodes(ctx, userID, purpose); err != nil {
		return CodeSent{}, err
	}
	if err := s.repo.createCode(ctx, codeRow{
		UserID: userID, Purpose: purpose, Method: method, PendingLoginID: pendingLoginID, Target: target,
		CodeHash: hashCode(code), BackupHashes: backupHashes, ExpiresAt: time.Now().Add(codeTTL),
	}); err != nil {
		return CodeSent{}, err
	}
	if err := s.deliverCode(ctx, purpose, method, target, code); err != nil {
		return CodeSent{}, apierror.Unavailable("the code could not be sent — try again in a minute")
	}
	sent := CodeSent{Method: method, SentTo: MaskEmail(target)}
	if method == MethodSMS {
		sent.SentTo = MaskPhone(target)
	}
	return sent, nil
}

func (s *Service) deliverCode(ctx context.Context, purpose, method, target, code string) error {
	if method == MethodSMS {
		return s.sms.SendSMS(ctx, target, fmt.Sprintf("FLOX code: %s. Valid for 10 minutes. Do not share it.", code))
	}
	subject := "Your FLOX sign-in code"
	what := "sign in to FLOX"
	if purpose == "enroll" {
		subject, what = "Confirm two-factor authentication in FLOX", "confirm email as your second factor in FLOX"
	}
	body := fmt.Sprintf("Your code to %s: %s\n\nIt is valid for 10 minutes. If you did not ask for it, change your password.\n\n"+
		"Ваш код FLOX: %s. Действует 10 минут. Если вы его не запрашивали, смените пароль.", what, code, code)
	return s.mailer.Send(ctx, target, subject, body)
}

// checkCode verifies code against row: true and marked used on a match;
// on a miss the attempt is counted and exhausted reports the code spent.
func (s *Service) checkCode(ctx context.Context, row codeRow, code string) (ok, exhausted bool, err error) {
	if subtle.ConstantTimeCompare([]byte(hashCode(code)), []byte(row.CodeHash)) == 1 {
		used, err := s.repo.useCode(ctx, row.ID)
		return used, false, err
	}
	attempts, err := s.repo.countAttempt(ctx, row.ID)
	return false, attempts >= codeAttempts, err
}

// --- login ---

// startCodeLogin sends a login code for a pending login of a user whose
// method is email or SMS.
func (s *Service) startCodeLogin(ctx context.Context, userID, pendingID string, st twoFactorState) (CodeSent, error) {
	target := st.Email
	if st.Method == MethodSMS {
		target = st.Phone
	}
	return s.sendCode(ctx, userID, "login", st.Method, target, pendingID, nil)
}

// ResendLoginCode sends a fresh code for a pending email/SMS login (the
// earlier ones stop working; within the cooldown nothing changes).
func (s *Service) ResendLoginCode(ctx context.Context, pendingToken string) (CodeSent, error) {
	pending, err := s.repo.FindPendingTOTPLogin(ctx, hashToken(pendingToken))
	if err != nil {
		return CodeSent{}, err
	}
	st, err := s.repo.twoFactorState(ctx, pending.UserID)
	if err != nil {
		return CodeSent{}, err
	}
	if pending.RequiresEnrollment || (st.Method != MethodEmail && st.Method != MethodSMS) {
		return CodeSent{}, apierror.Validation("this sign-in does not use a sent code", nil)
	}
	return s.startCodeLogin(ctx, pending.UserID, pending.ID, st)
}

// verifyCodeLogin checks a sent code (or a backup code) for a pending
// login. Three wrong guesses spend the code and the pending login.
func (s *Service) verifyCodeLogin(ctx context.Context, pending pendingTOTPLoginRow, code string) error {
	if consumed, err := s.repo.ConsumeBackupCode(ctx, pending.UserID, totpengine.HashBackupCode(code)); err != nil || consumed {
		return err
	}
	row, found, err := s.repo.latestCode(ctx, pending.UserID, "login", pending.ID)
	if err != nil {
		return err
	}
	if !found {
		_ = s.repo.DeletePendingTOTPLogin(ctx, pending.ID)
		return apierror.Unauthorized("the code has expired — sign in again")
	}
	ok, exhausted, err := s.checkCode(ctx, row, code)
	if err != nil || ok {
		return err
	}
	// A wrong code spends the same per-account budget as a wrong password.
	if err := s.repo.RecordLoginFailureForUser(ctx, pending.UserID); err != nil {
		return err
	}
	if exhausted {
		_ = s.repo.DeletePendingTOTPLogin(ctx, pending.ID)
		return apierror.Unauthorized("too many wrong codes — sign in again")
	}
	return apierror.Unauthorized("incorrect code")
}

// --- enrollment ---

// EnrollCodeInput sets up email or SMS: Phone for SMS (E.164).
type EnrollCodeInput struct {
	Method string
	Phone  string
}

// CodeEnrollment is the started setup: where the code went and the backup
// codes that become the user's once it is confirmed.
type CodeEnrollment struct {
	CodeSent
	BackupCodes []string `json:"backupCodes"`
}

// EnrollCode starts setting up email or SMS for userID: sends a code to
// the account's email or to the given phone. The current method stays
// until the code is confirmed.
func (s *Service) EnrollCode(ctx context.Context, userID string, in EnrollCodeInput) (CodeEnrollment, error) {
	st, err := s.repo.twoFactorState(ctx, userID)
	if err != nil {
		return CodeEnrollment{}, err
	}
	target := st.Email
	switch in.Method {
	case MethodEmail:
	case MethodSMS:
		if !s.SMSAvailable() {
			return CodeEnrollment{}, apierror.Unavailable("SMS codes are not available on this deployment")
		}
		target = NormalizePhone(in.Phone)
		if !phonePattern.MatchString(target) {
			return CodeEnrollment{}, apierror.Validation("invalid phone", map[string]string{"phone": "an international number, e.g. +79001234567"})
		}
		if err := s.repo.setPendingPhone(ctx, userID, target); err != nil {
			return CodeEnrollment{}, err
		}
	default:
		return CodeEnrollment{}, apierror.Validation("invalid method", map[string]string{"method": "email or sms"})
	}
	backup, err := totpengine.GenerateBackupCodes()
	if err != nil {
		return CodeEnrollment{}, err
	}
	hashes := make([]string, len(backup))
	for i, c := range backup {
		hashes[i] = totpengine.HashBackupCode(c)
	}
	if err := s.repo.expireCodes(ctx, userID, "enroll"); err != nil {
		return CodeEnrollment{}, err
	}
	sent, err := s.sendCode(ctx, userID, "enroll", in.Method, target, "", hashes)
	if err != nil {
		return CodeEnrollment{}, err
	}
	return CodeEnrollment{CodeSent: sent, BackupCodes: backup}, nil
}

// ConfirmCode finishes EnrollCode: the code makes its method the user's
// (replacing an app, if any) with the enrollment's backup codes.
func (s *Service) ConfirmCode(ctx context.Context, userID, code string) (string, error) {
	row, found, err := s.repo.latestCode(ctx, userID, "enroll", "")
	if err != nil {
		return "", err
	}
	if !found {
		return "", apierror.Validation("the code has expired — send a new one", map[string]string{"code": "expired"})
	}
	ok, exhausted, err := s.checkCode(ctx, row, code)
	if err != nil {
		return "", err
	}
	if !ok {
		if exhausted {
			return "", apierror.Validation("too many wrong codes — send a new one", map[string]string{"code": "too many attempts"})
		}
		return "", apierror.Validation("incorrect code", map[string]string{"code": "incorrect"})
	}
	if err := s.repo.setCodeMethod(ctx, userID, row.Method, row.BackupHashes); err != nil {
		return "", err
	}
	return row.Method, nil
}

// CompleteCodeEnrollmentLogin is ConfirmCode for a login that required
// setting up 2FA: confirming also signs in.
func (s *Service) CompleteCodeEnrollmentLogin(ctx context.Context, pendingToken, code string) (Result, error) {
	pending, err := s.repo.FindPendingTOTPLogin(ctx, hashToken(pendingToken))
	if err != nil {
		return Result{}, err
	}
	if !pending.RequiresEnrollment {
		return Result{}, apierror.Validation("already enrolled — enter your code instead", nil)
	}
	method, err := s.ConfirmCode(ctx, pending.UserID, code)
	if err != nil {
		return Result{}, err
	}
	s.recordActivity(ctx, pending.OrganizationID, pending.UserID, "security.two_factor_enabled."+method, "user", pending.UserID)
	return s.finishPendingLogin(ctx, pending)
}

// finishPendingLogin turns a pending login into a session.
func (s *Service) finishPendingLogin(ctx context.Context, pending pendingTOTPLoginRow) (Result, error) {
	who, err := s.repo.WhoAmI(ctx, pending.UserID, pending.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	sec, err := s.repo.GetSecuritySettings(ctx, pending.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	_ = s.repo.DeletePendingTOTPLogin(ctx, pending.ID)
	return s.createSession(ctx, who.UserID, pending.OrganizationID, who.UserName, who.UserEmail, who.RoleKey, who.OrganizationName, sec.SessionTimeoutMinutes)
}

// --- repository ---

// attachLiveLoginCode moves the user's live login code (sent moments ago
// for an earlier sign-in) to a new pending login: a second sign-in within
// the resend cooldown keeps using the code already on its way.
func (r *Repository) attachLiveLoginCode(ctx context.Context, userID, pendingLoginID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE two_factor_codes SET pending_login_id = $2
		WHERE id = (SELECT id FROM two_factor_codes
		            WHERE user_id = $1 AND purpose = 'login' AND used_at IS NULL AND expires_at > now()
		            ORDER BY created_at DESC LIMIT 1)`, userID, pendingLoginID)
	if err != nil {
		return fmt.Errorf("auth: reusing login code: %w", err)
	}
	return nil
}

type twoFactorState struct {
	Method string // "" = none
	Email  string
	Phone  string
}

func (r *Repository) twoFactorState(ctx context.Context, userID string) (twoFactorState, error) {
	var st twoFactorState
	var method *string
	err := r.db.QueryRow(ctx, `SELECT two_factor_method, email, phone FROM users WHERE id = $1`, userID).Scan(&method, &st.Email, &st.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return twoFactorState{}, apierror.NotFound("user not found")
	}
	if err != nil {
		return twoFactorState{}, fmt.Errorf("auth: reading two-factor state: %w", err)
	}
	if method != nil {
		st.Method = *method
	}
	return st, nil
}

type codeRow struct {
	ID             string
	UserID         string
	Purpose        string
	Method         string
	PendingLoginID string
	Target         string
	CodeHash       string
	BackupHashes   []string
	ExpiresAt      time.Time
}

func (r *Repository) createCode(ctx context.Context, c codeRow) error {
	var pending *string
	if c.PendingLoginID != "" {
		pending = &c.PendingLoginID
	}
	hashes := c.BackupHashes
	if hashes == nil {
		hashes = []string{}
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO two_factor_codes (id, user_id, purpose, method, pending_login_id, target, code_hash, backup_code_hashes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		idgen.New(), c.UserID, c.Purpose, c.Method, pending, c.Target, c.CodeHash, hashes, c.ExpiresAt)
	if err != nil {
		return fmt.Errorf("auth: storing code: %w", err)
	}
	return nil
}

// recentCodeSends is the user's last code of purpose sent (the cooldown is
// per purpose: setting up a method, then signing in, is two codes) and how
// many codes of any purpose since since (the hourly cap is per user).
func (r *Repository) recentCodeSends(ctx context.Context, userID, purpose string, since time.Time) (last time.Time, count int, err error) {
	var lastAt *time.Time
	err = r.db.QueryRow(ctx, `
		SELECT max(created_at) FILTER (WHERE purpose = $3), count(*) FILTER (WHERE created_at > $2)
		FROM two_factor_codes WHERE user_id = $1`,
		userID, since, purpose).Scan(&lastAt, &count)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("auth: counting sent codes: %w", err)
	}
	if lastAt != nil {
		last = *lastAt
	}
	return last, count, nil
}

// latestCode is the newest live code of purpose (for a login: of that
// pending login): not used, not expired, attempts left.
func (r *Repository) latestCode(ctx context.Context, userID, purpose, pendingLoginID string) (codeRow, bool, error) {
	var c codeRow
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, purpose, method, target, code_hash, backup_code_hashes, expires_at
		FROM two_factor_codes
		WHERE user_id = $1 AND purpose = $2 AND ($3 = '' OR pending_login_id = $3::text)
		  AND used_at IS NULL AND expires_at > now() AND attempts < $4
		ORDER BY created_at DESC LIMIT 1`, userID, purpose, pendingLoginID, codeAttempts,
	).Scan(&c.ID, &c.UserID, &c.Purpose, &c.Method, &c.Target, &c.CodeHash, &c.BackupHashes, &c.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return codeRow{}, false, nil
	}
	if err != nil {
		return codeRow{}, false, fmt.Errorf("auth: reading code: %w", err)
	}
	return c, true, nil
}

// useCode marks a code used, once.
func (r *Repository) useCode(ctx context.Context, id string) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE two_factor_codes SET used_at = now() WHERE id = $1 AND used_at IS NULL`, id)
	if err != nil {
		return false, fmt.Errorf("auth: using code: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) countAttempt(ctx context.Context, id string) (int, error) {
	var n int
	if err := r.db.QueryRow(ctx, `UPDATE two_factor_codes SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts`, id).Scan(&n); err != nil {
		return 0, fmt.Errorf("auth: counting attempt: %w", err)
	}
	return n, nil
}

// expireCodes ends the user's live codes of purpose (a new one replaces
// them).
func (r *Repository) expireCodes(ctx context.Context, userID, purpose string) error {
	_, err := r.db.Exec(ctx, `UPDATE two_factor_codes SET expires_at = now() WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()`, userID, purpose)
	if err != nil {
		return fmt.Errorf("auth: expiring codes: %w", err)
	}
	return nil
}

func (r *Repository) setPendingPhone(ctx context.Context, userID, phone string) error {
	if _, err := r.db.Exec(ctx, `UPDATE users SET phone_pending = $2 WHERE id = $1`, userID, phone); err != nil {
		return fmt.Errorf("auth: storing phone: %w", err)
	}
	return nil
}

// setCodeMethod makes email or SMS the user's method (SMS: the pending
// phone becomes the phone), drops a TOTP secret, and replaces the backup
// codes — one transaction.
func (r *Repository) setCodeMethod(ctx context.Context, userID, method string, backupHashes []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: beginning set-method tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if method == MethodSMS {
		_, err = tx.Exec(ctx, `UPDATE users SET two_factor_method = 'sms', phone = phone_pending, phone_pending = '', totp_secret = NULL, totp_enabled_at = NULL WHERE id = $1`, userID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE users SET two_factor_method = 'email', phone = '', phone_pending = '', totp_secret = NULL, totp_enabled_at = NULL WHERE id = $1`, userID)
	}
	if err != nil {
		return fmt.Errorf("auth: setting two-factor method: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_totp_backup_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("auth: clearing backup codes: %w", err)
	}
	for _, h := range backupHashes {
		if _, err := tx.Exec(ctx, `INSERT INTO user_totp_backup_codes (id, user_id, code_hash) VALUES ($1, $2, $3)`, idgen.New(), userID, h); err != nil {
			return fmt.Errorf("auth: inserting backup code: %w", err)
		}
	}
	return tx.Commit(ctx)
}
