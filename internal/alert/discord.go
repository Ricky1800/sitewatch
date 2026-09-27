package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// discordColor mirrors Discord's embed color field (a decimal RGB integer).
const (
	discordColorRed    = 0xE74C3C
	discordColorGreen  = 0x2ECC71
	discordColorYellow = 0xF1C40F
)

// DiscordPayload is the JSON body posted to a Discord incoming webhook. Field
// names and shape follow Discord's documented webhook execute format.
type DiscordPayload struct {
	Username string         `json:"username"`
	Embeds   []DiscordEmbed `json:"embeds"`
}

// DiscordEmbed is a single Discord embed object.
type DiscordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color"`
	Fields      []DiscordField `json:"fields,omitempty"`
}

// DiscordField is a Discord embed field.
type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// BuildDiscordPayload builds the Discord webhook payload for ev. It is
// exported (and separate from the HTTP send) so golden tests can assert on
// the exact JSON shape without a network round trip.
func BuildDiscordPayload(ev Event) DiscordPayload {
	color := discordColorYellow
	switch ev.Kind {
	case Down:
		color = discordColorRed
	case Up:
		color = discordColorGreen
	case SSLExpiry:
		color = discordColorYellow
	}

	embed := DiscordEmbed{
		Title:       summary(ev),
		Description: detail(ev),
		Color:       color,
		Fields: []DiscordField{
			{Name: "Check", Value: ev.CheckName, Inline: true},
			{Name: "URL", Value: ev.URL, Inline: true},
			{Name: "Event", Value: ev.Kind.String(), Inline: true},
		},
	}

	return DiscordPayload{
		Username: "sitewatch",
		Embeds:   []DiscordEmbed{embed},
	}
}

// DiscordNotifier posts alerts to a Discord incoming webhook URL.
type DiscordNotifier struct {
	WebhookURL string
	Client     *http.Client
}

// Name implements Notifier.
func (n *DiscordNotifier) Name() string { return "discord" }

// Notify implements Notifier.
func (n *DiscordNotifier) Notify(ctx context.Context, ev Event) error {
	payload := BuildDiscordPayload(ev)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: marshal payload: %w", err)
	}
	return postJSON(ctx, n.client(), n.WebhookURL, body, "discord")
}

func (n *DiscordNotifier) client() *http.Client {
	if n.Client != nil {
		return n.Client
	}
	return http.DefaultClient
}

// postJSON is shared by every webhook-style notifier (Discord, Slack,
// generic webhook): POST body as application/json and treat any non-2xx
// response as an error.
func postJSON(ctx context.Context, client *http.Client, url string, body []byte, channel string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: build request: %w", channel, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: request failed: %w", channel, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: unexpected status %d", channel, resp.StatusCode)
	}
	return nil
}
