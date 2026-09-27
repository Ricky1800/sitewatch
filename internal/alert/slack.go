package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SlackPayload is the JSON body posted to a Slack incoming webhook, using
// Slack's simple "text" + attachments format for broad workspace
// compatibility (no Block Kit app install required).
type SlackPayload struct {
	Text        string            `json:"text"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackAttachment is a single Slack legacy-attachment object.
type SlackAttachment struct {
	Color  string       `json:"color"`
	Text   string       `json:"text,omitempty"`
	Fields []SlackField `json:"fields,omitempty"`
}

// SlackField is a Slack attachment field.
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// BuildSlackPayload builds the Slack webhook payload for ev.
func BuildSlackPayload(ev Event) SlackPayload {
	color := "warning"
	switch ev.Kind {
	case Down:
		color = "danger"
	case Up:
		color = "good"
	case SSLExpiry:
		color = "warning"
	}

	return SlackPayload{
		Text: summary(ev),
		Attachments: []SlackAttachment{
			{
				Color: color,
				Text:  detail(ev),
				Fields: []SlackField{
					{Title: "Check", Value: ev.CheckName, Short: true},
					{Title: "URL", Value: ev.URL, Short: true},
					{Title: "Event", Value: ev.Kind.String(), Short: true},
				},
			},
		},
	}
}

// SlackNotifier posts alerts to a Slack incoming webhook URL.
type SlackNotifier struct {
	WebhookURL string
	Client     *http.Client
}

// Name implements Notifier.
func (n *SlackNotifier) Name() string { return "slack" }

// Notify implements Notifier.
func (n *SlackNotifier) Notify(ctx context.Context, ev Event) error {
	payload := BuildSlackPayload(ev)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack: marshal payload: %w", err)
	}
	return postJSON(ctx, n.client(), n.WebhookURL, body, "slack")
}

func (n *SlackNotifier) client() *http.Client {
	if n.Client != nil {
		return n.Client
	}
	return http.DefaultClient
}
