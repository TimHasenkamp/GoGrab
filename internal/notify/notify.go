// Package notify dispatches notifications when secret-request state changes.
//
// v1 supports a single generic webhook target (POST JSON). Channels like
// Slack/Discord/n8n all accept a JSON POST and can be wired up by pointing
// GOGRAB_NOTIFY_WEBHOOK_URL at them. Dispatch is fire-and-forget on a
// background goroutine so it never blocks the request path.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Event is the JSON shape sent to the webhook URL.
type Event struct {
	Type        string    `json:"type"`         // e.g. "request.submitted"
	RequestID   string    `json:"request_id"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"created_by"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// Notifier sends Events. Implementations must be safe for concurrent use.
type Notifier interface {
	Dispatch(ctx context.Context, evt Event)
}

// Nop is a no-op notifier used when no webhook is configured.
type Nop struct{}

func (Nop) Dispatch(_ context.Context, _ Event) {}

// Webhook is the v1 dispatcher. It does not retry; failures are logged.
type Webhook struct {
	url    string
	client *http.Client
	log    *slog.Logger
}

// NewWebhook returns a Webhook notifier. timeout is per-request.
func NewWebhook(url string, timeout time.Duration, log *slog.Logger) *Webhook {
	return &Webhook{
		url:    url,
		client: &http.Client{Timeout: timeout},
		log:    log,
	}
}

// Dispatch sends evt on a background goroutine using a detached context so
// request cancellation doesn't kill the notification.
func (w *Webhook) Dispatch(_ context.Context, evt Event) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), w.client.Timeout)
		defer cancel()
		if err := w.send(ctx, evt); err != nil {
			w.log.Warn("notify webhook failed", "err", err, "type", evt.Type, "request_id", evt.RequestID)
		}
	}()
}

func (w *Webhook) send(ctx context.Context, evt Event) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gograb-webhook/1")
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
