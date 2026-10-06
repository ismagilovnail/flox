// Package errtrack sends errors and panics to an error tracker (audit
// 2026-10-06, §14: there was none — a panic was a log line among millions).
//
// Behind Reporter (CLAUDE.md #11). The implementation speaks the Sentry
// protocol, which Sentry itself and compatible self-hosted trackers
// (GlitchTip, …) accept; it is enabled by SENTRY_DSN and is a no-op
// without it. Reporting is asynchronous — the SDK queues and sends in the
// background, dropping when full — so nothing on the redirect path waits
// on it.
//
// What is reported:
//   - every log record at ERROR level, from every binary (Handler wraps the
//     slog handler), with its attributes — no call site has to remember;
//   - panics in HTTP handlers (Middleware) and in background loops (Go).
package errtrack

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
)

// Reporter is an error tracker.
type Reporter interface {
	// Report sends one error event: msg, the error value if any, and
	// key/value context.
	Report(msg string, err error, attrs map[string]string)
	// Flush waits up to timeout for queued events to be sent.
	Flush(timeout time.Duration)
}

// Noop reports nothing (no DSN configured).
type Noop struct{}

func (Noop) Report(string, error, map[string]string) {}
func (Noop) Flush(time.Duration)                     {}

// Config selects and labels the tracker.
type Config struct {
	DSN         string // empty: Noop
	Environment string
	Service     string // api, tracker, worker — a tag on every event
}

// New returns the configured Reporter.
func New(cfg Config) (Reporter, error) {
	if cfg.DSN == "" {
		return Noop{}, nil
	}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:         cfg.DSN,
		Environment: cfg.Environment,
		// Events carry what the log line carries; nothing else about the
		// request (visitor IPs, headers) is attached.
		SendDefaultPII: false,
		// sentry-go 0.49's newer telemetry buffer can lose an event queued
		// right before Flush (seen in tests: Flush reports success, nothing
		// is sent) — exactly the panic-then-exit case Go relies on. The
		// classic transport flushes what was queued.
		DisableTelemetryBuffer: true,
	})
	if err != nil {
		return nil, fmt.Errorf("errtrack: configuring the tracker: %w", err)
	}
	scope := sentry.NewScope()
	scope.SetTag("service", cfg.Service)
	return &sentryReporter{hub: sentry.NewHub(client, scope)}, nil
}

type sentryReporter struct {
	hub *sentry.Hub
}

func (r *sentryReporter) Report(msg string, err error, attrs map[string]string) {
	ev := sentry.NewEvent()
	ev.Level = sentry.LevelError
	ev.Message = msg
	if err != nil {
		ev.Exception = []sentry.Exception{{Type: fmt.Sprintf("%T", err), Value: err.Error()}}
	}
	if len(attrs) > 0 {
		ctx := make(map[string]any, len(attrs))
		for k, v := range attrs {
			ctx[k] = v
		}
		ev.Contexts = map[string]sentry.Context{"log": ctx}
	}
	r.hub.CaptureEvent(ev)
}

func (r *sentryReporter) Flush(timeout time.Duration) { r.hub.Flush(timeout) }

// Handler forwards every record at ERROR or above to r, then to next.
func Handler(next slog.Handler, r Reporter) slog.Handler {
	if _, off := r.(Noop); off {
		return next
	}
	return &handler{next: next, r: r}
}

type handler struct {
	next  slog.Handler
	r     Reporter
	attrs []slog.Attr
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

func (h *handler) Handle(ctx context.Context, rec slog.Record) error {
	if rec.Level >= slog.LevelError {
		attrs := map[string]string{}
		var err error
		add := func(a slog.Attr) bool {
			if e, ok := a.Value.Any().(error); ok && a.Key == "error" {
				err = e
			}
			attrs[a.Key] = a.Value.String()
			return true
		}
		for _, a := range h.attrs {
			add(a)
		}
		rec.Attrs(add)
		h.r.Report(rec.Message, err, attrs)
	}
	return h.next.Handle(ctx, rec)
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	return &handler{next: h.next.WithAttrs(as), r: h.r, attrs: append(append([]slog.Attr{}, h.attrs...), as...)}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{next: h.next.WithGroup(name), r: h.r, attrs: h.attrs}
}

// Middleware recovers a panicking handler: reports it with its stack and
// the route, logs it, and answers 500 — replacing chi's Recoverer.
func Middleware(r Reporter, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() {
				p := recover()
				if p == nil {
					return
				}
				if p == http.ErrAbortHandler {
					panic(p) // net/http's own "abort the response" signal, not a bug
				}
				stack := string(debug.Stack())
				r.Report("panic in http handler", fmt.Errorf("panic: %v", p),
					map[string]string{"method": req.Method, "path": req.URL.Path, "stack": stack})
				logger.Error("panic in http handler", "panic", fmt.Sprint(p), "path", req.URL.Path, "stack", stack)
				w.WriteHeader(http.StatusInternalServerError)
			}()
			next.ServeHTTP(w, req)
		})
	}
}

// Go runs fn, reporting a panic (with its stack, flushed) before letting it
// crash the process as it would have — a background loop that panicked is
// not silently restarted.
func Go(r Reporter, name string, fn func()) {
	defer func() {
		if p := recover(); p != nil {
			r.Report("panic in "+name, fmt.Errorf("panic: %v", p), map[string]string{"stack": string(debug.Stack())})
			r.Flush(5 * time.Second)
			panic(p)
		}
	}()
	fn()
}
