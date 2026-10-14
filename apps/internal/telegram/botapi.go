package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BotAPI is the part of the Telegram Bot API apps/api calls when a bot is
// connected or removed (§74: behind an interface; tests use a fake server).
// The tracker never calls it: webhook answers ride in the HTTP response.
type BotAPI interface {
	GetMe(ctx context.Context, token string) (User, error)
	SetWebhook(ctx context.Context, token, webhookURL, secret string, allowedUpdates []string) error
	DeleteWebhook(ctx context.Context, token string) error
	GetChat(ctx context.Context, token, chat string) (Chat, error)
	GetChatMember(ctx context.Context, token string, chatID, userID int64) (ChatMember, error)
	CreateChatInviteLink(ctx context.Context, token string, chatID int64, name string) (string, error)
}

// HTTPBotAPI calls the Bot API at BaseURL (https://api.telegram.org).
type HTTPBotAPI struct {
	BaseURL string
	Client  *http.Client
}

func NewHTTPBotAPI(baseURL string) *HTTPBotAPI {
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	return &HTTPBotAPI{BaseURL: strings.TrimRight(baseURL, "/"), Client: &http.Client{Timeout: 10 * time.Second}}
}

// APIError is a call Telegram answered with ok:false — Description is its
// own explanation, safe to show the operator ("Unauthorized", "chat not
// found", …).
type APIError struct {
	Method      string
	Code        int
	Description string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
}

// call posts params to method and decodes the result into out. The token
// is in the URL path, so a transport error is reported without the URL —
// it must never reach a log or a response.
func (a *HTTPBotAPI) call(ctx context.Context, token, method string, params any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/bot"+token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram %s: building request", method)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.Client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()
	var env struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&env); err != nil {
		return fmt.Errorf("telegram %s: reading answer (HTTP %d)", method, resp.StatusCode)
	}
	if !env.OK {
		return &APIError{Method: method, Code: env.ErrorCode, Description: env.Description}
	}
	if out != nil {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("telegram %s: decoding result: %w", method, err)
		}
	}
	return nil
}

func (a *HTTPBotAPI) GetMe(ctx context.Context, token string) (User, error) {
	var u User
	err := a.call(ctx, token, "getMe", struct{}{}, &u)
	return u, err
}

func (a *HTTPBotAPI) SetWebhook(ctx context.Context, token, webhookURL, secret string, allowedUpdates []string) error {
	return a.call(ctx, token, "setWebhook", map[string]any{
		"url": webhookURL, "secret_token": secret, "allowed_updates": allowedUpdates, "drop_pending_updates": true,
	}, nil)
}

func (a *HTTPBotAPI) DeleteWebhook(ctx context.Context, token string) error {
	return a.call(ctx, token, "deleteWebhook", struct{}{}, nil)
}

func (a *HTTPBotAPI) GetChat(ctx context.Context, token, chat string) (Chat, error) {
	var c Chat
	err := a.call(ctx, token, "getChat", map[string]any{"chat_id": chat}, &c)
	return c, err
}

func (a *HTTPBotAPI) GetChatMember(ctx context.Context, token string, chatID, userID int64) (ChatMember, error) {
	var m ChatMember
	err := a.call(ctx, token, "getChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, &m)
	return m, err
}

func (a *HTTPBotAPI) CreateChatInviteLink(ctx context.Context, token string, chatID int64, name string) (string, error) {
	var link struct {
		InviteLink string `json:"invite_link"`
	}
	err := a.call(ctx, token, "createChatInviteLink", map[string]any{"chat_id": chatID, "name": name}, &link)
	return link.InviteLink, err
}

// Bot API types — only the fields FLOX reads.

type User struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type ChatMember struct {
	Status         string `json:"status"`
	CanInviteUsers bool   `json:"can_invite_users"`
	User           User   `json:"user"`
}

// Update is one webhook call.
type Update struct {
	UpdateID        int64              `json:"update_id"`
	Message         *Message           `json:"message"`
	ChatMember      *ChatMemberUpdated `json:"chat_member"`
	ChatJoinRequest *ChatJoinRequest   `json:"chat_join_request"`
}

type Message struct {
	Chat Chat   `json:"chat"`
	From *User  `json:"from"`
	Text string `json:"text"`
}

type ChatMemberUpdated struct {
	Chat          Chat       `json:"chat"`
	OldChatMember ChatMember `json:"old_chat_member"`
	NewChatMember ChatMember `json:"new_chat_member"`
}

type ChatJoinRequest struct {
	Chat Chat `json:"chat"`
	From User `json:"from"`
}

// AllowedUpdates is what the webhook asks Telegram for. chat_member must
// be named explicitly: Telegram leaves it out by default.
var AllowedUpdates = []string{"message", "chat_member", "chat_join_request"}
