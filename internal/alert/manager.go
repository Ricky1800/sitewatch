package alert

import (
	"context"
	"net/http"
	"time"

	"github.com/Ricky1800/sitewatch/internal/config"
)

// Manager fans a single Event out to every configured Notifier. A failure
// sending to one channel does not stop the others.
type Manager struct {
	Notifiers []Notifier
	// Logf receives one line per delivery failure. Defaults to a no-op.
	Logf func(format string, args ...any)
}

// NewManager builds a Manager from the alerts section of a loaded config.
// Channels left unconfigured are simply omitted.
func NewManager(cfg config.Alerts) *Manager {
	client := &http.Client{Timeout: 10 * time.Second}
	var notifiers []Notifier

	if cfg.Discord != nil {
		notifiers = append(notifiers, &DiscordNotifier{WebhookURL: cfg.Discord.WebhookURL, Client: client})
	}
	if cfg.Slack != nil {
		notifiers = append(notifiers, &SlackNotifier{WebhookURL: cfg.Slack.WebhookURL, Client: client})
	}
	if cfg.Webhook != nil {
		notifiers = append(notifiers, &WebhookNotifier{URL: cfg.Webhook.URL, Headers: cfg.Webhook.Headers, Client: client})
	}
	if cfg.Email != nil {
		notifiers = append(notifiers, &EmailNotifier{
			SMTPHost: cfg.Email.SMTPHost,
			SMTPPort: cfg.Email.SMTPPort,
			Username: cfg.Email.Username,
			Password: cfg.Email.Password(),
			From:     cfg.Email.From,
			To:       cfg.Email.To,
		})
	}

	return &Manager{Notifiers: notifiers}
}

// Send delivers ev to every configured notifier, logging (but not
// returning) individual failures so one broken channel can't block others.
func (m *Manager) Send(ctx context.Context, ev Event) {
	logf := m.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	for _, n := range m.Notifiers {
		if err := n.Notify(ctx, ev); err != nil {
			logf("alert: %s: %v", n.Name(), err)
		}
	}
}
