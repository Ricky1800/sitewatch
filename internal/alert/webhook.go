package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookPayload is the JSON body posted to a generic webhook, designed to
// be easy for a small script or automation platform (e.g. n8n, Zapier) to
// consume without Discord/Slack-specific shape knowledge.
type WebhookPayload struct {
	Event            string     `json:"event"`
	Check            string     `json:"check"`
	URL              string     `json:"url"`
	Time             time.Time  `json:"time"`
	Message          string     `json:"message"`
	ConsecutiveFails int        `json:"consecutive_fails,omitempty"`
	Error            string     `json:"error,omitempty"`
	DowntimeSeconds  float64    `json:"downtime_seconds,omitempty"`
	SSLExpiry        *time.Time `json:"ssl_expiry,omitempty"`
	SSLDaysLeft      int        `json:"ssl_days_left,omitempty"`
}

// BuildWebhookPayload builds the generic webhook payload for ev.
func BuildWebhookPayload(ev Event) WebhookPayload {
	p := WebhookPayload{
		Event:   ev.Kind.String(),
		Check:   ev.CheckName,
		URL:     ev.URL,
		Time:    ev.Time,
		Message: summary(ev),
	}
	switch ev.Kind {
	case Down:
		p.ConsecutiveFails = ev.ConsecutiveFails
		p.Error = ev.Error
	case Up:
		p.DowntimeSeconds = ev.Downtime.Seconds()
	case SSLExpiry:
		exp := ev.SSLExpiry
		p.SSLExpiry = &exp
		p.SSLDaysLeft = ev.SSLDaysLeft
	}
	return p
}

// WebhookNotifier posts a generic JSON payload to any URL, with optional
// extra headers (for a bearer token, a shared secret, etc).
type WebhookNotifier struct {
	URL     string
	Headers map[string]string
	Client  *http.Client
}

// Name implements Notifier.
func (n *WebhookNotifier) Name() string { return "webhook" }

// Notify implements Notifier.
func (n *WebhookNotifier) Notify(ctx context.Context, ev Event) error {
	payload := BuildWebhookPayload(ev)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.Headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client().Do(req)
	if err != nil {
		return fmt.Errorf("webhook: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (n *WebhookNotifier) client() *http.Client {
	if n.Client != nil {
		return n.Client
	}
	return http.DefaultClient
}
