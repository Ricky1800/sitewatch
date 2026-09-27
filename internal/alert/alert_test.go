package alert

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var update = os.Getenv("SITEWATCH_UPDATE_GOLDEN") == "1"

var errFakeSMTP = errors.New("fake smtp failure")

func fixedEvent(kind Kind) Event {
	tm := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	switch kind {
	case Down:
		return Event{
			Kind:             Down,
			CheckName:        "Acme Homepage",
			URL:              "https://acme.example.com",
			Time:             tm,
			ConsecutiveFails: 3,
			Error:            "dial tcp: connection refused",
		}
	case Up:
		return Event{
			Kind:      Up,
			CheckName: "Acme Homepage",
			URL:       "https://acme.example.com",
			Time:      tm,
			Downtime:  5*time.Minute + 30*time.Second,
		}
	default:
		return Event{
			Kind:        SSLExpiry,
			CheckName:   "Acme Homepage",
			URL:         "https://acme.example.com",
			Time:        tm,
			SSLExpiry:   tm.Add(9 * 24 * time.Hour),
			SSLDaysLeft: 9,
		}
	}
}

// goldenJSON compares got (any JSON-marshalable value) against a golden
// fixture file, rewriting it when SITEWATCH_UPDATE_GOLDEN=1 is set.
func goldenJSON(t *testing.T, name string, got any) {
	t.Helper()
	gotBytes, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join("testdata", name)
	if update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, append(gotBytes, '\n'), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with SITEWATCH_UPDATE_GOLDEN=1 to create it): %v", path, err)
	}
	if string(want) != string(gotBytes)+"\n" {
		t.Errorf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, gotBytes, want)
	}
}

func goldenText(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with SITEWATCH_UPDATE_GOLDEN=1 to create it): %v", path, err)
	}
	if string(want) != got {
		t.Errorf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestDiscordPayload_Down_Golden(t *testing.T) {
	goldenJSON(t, "discord_down.json", BuildDiscordPayload(fixedEvent(Down)))
}

func TestDiscordPayload_Up_Golden(t *testing.T) {
	goldenJSON(t, "discord_up.json", BuildDiscordPayload(fixedEvent(Up)))
}

func TestDiscordPayload_SSLExpiry_Golden(t *testing.T) {
	goldenJSON(t, "discord_ssl.json", BuildDiscordPayload(fixedEvent(SSLExpiry)))
}

func TestSlackPayload_Down_Golden(t *testing.T) {
	goldenJSON(t, "slack_down.json", BuildSlackPayload(fixedEvent(Down)))
}

func TestSlackPayload_Up_Golden(t *testing.T) {
	goldenJSON(t, "slack_up.json", BuildSlackPayload(fixedEvent(Up)))
}

func TestWebhookPayload_Down_Golden(t *testing.T) {
	goldenJSON(t, "webhook_down.json", BuildWebhookPayload(fixedEvent(Down)))
}

func TestWebhookPayload_Up_Golden(t *testing.T) {
	goldenJSON(t, "webhook_up.json", BuildWebhookPayload(fixedEvent(Up)))
}

func TestWebhookPayload_SSLExpiry_Golden(t *testing.T) {
	goldenJSON(t, "webhook_ssl.json", BuildWebhookPayload(fixedEvent(SSLExpiry)))
}

func TestEmailMessage_Down_Golden(t *testing.T) {
	msg := BuildEmailMessage("alerts@sitewatch.example", []string{"oncall@example.com"}, fixedEvent(Down))
	goldenText(t, "email_down.txt", msg)
}

func TestEmailMessage_Up_Golden(t *testing.T) {
	msg := BuildEmailMessage("alerts@sitewatch.example", []string{"oncall@example.com"}, fixedEvent(Up))
	goldenText(t, "email_up.txt", msg)
}

// --- Delivery tests (real HTTP round trip against httptest, no golden) ---

func TestDiscordNotifier_Notify_PostsJSON(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		buf, _ := io.ReadAll(r.Body)
		gotBody = buf
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := &DiscordNotifier{WebhookURL: srv.URL, Client: srv.Client()}
	if err := n.Notify(context.Background(), fixedEvent(Down)); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected application/json content type, got %q", gotContentType)
	}
	var payload DiscordPayload
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("server received invalid JSON: %v (%s)", err, gotBody)
	}
	if len(payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(payload.Embeds))
	}
}

func TestDiscordNotifier_Notify_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	n := &DiscordNotifier{WebhookURL: srv.URL, Client: srv.Client()}
	if err := n.Notify(context.Background(), fixedEvent(Down)); err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
}

func TestSlackNotifier_Notify_PostsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload SlackPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("invalid JSON from slack notifier: %v", err)
		}
		if payload.Text == "" {
			t.Error("expected non-empty top-level text")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &SlackNotifier{WebhookURL: srv.URL, Client: srv.Client()}
	if err := n.Notify(context.Background(), fixedEvent(Up)); err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

func TestWebhookNotifier_Notify_SendsHeadersAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-Key"); got != "secret123" {
			t.Errorf("expected custom header to be forwarded, got %q", got)
		}
		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("invalid JSON: %v", err)
		}
		if payload.Event != "DOWN" {
			t.Errorf("expected event DOWN, got %q", payload.Event)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &WebhookNotifier{URL: srv.URL, Headers: map[string]string{"X-Api-Key": "secret123"}, Client: srv.Client()}
	if err := n.Notify(context.Background(), fixedEvent(Down)); err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

func TestManager_Send_FansOutAndSurvivesOneFailure(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	var logged []string
	m := &Manager{
		Notifiers: []Notifier{
			&DiscordNotifier{WebhookURL: failSrv.URL, Client: failSrv.Client()},
			&SlackNotifier{WebhookURL: okSrv.URL, Client: okSrv.Client()},
		},
		Logf: func(format string, args ...any) {
			logged = append(logged, format)
		},
	}
	m.Send(context.Background(), fixedEvent(Down))
	if len(logged) != 1 {
		t.Fatalf("expected exactly 1 logged failure (discord), got %d: %v", len(logged), logged)
	}
}

// --- Email: injected smtp.SendMail double, no real network/server ---

func TestEmailNotifier_Notify_CallsSendWithExpectedArgs(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte

	n := &EmailNotifier{
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
		Username: "user",
		Password: "pw",
		From:     "alerts@sitewatch.example",
		To:       []string{"oncall@example.com"},
		send: func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
			gotAddr = addr
			gotFrom = from
			gotTo = to
			gotMsg = msg
			return nil
		},
	}
	if err := n.Notify(context.Background(), fixedEvent(Down)); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotAddr != "smtp.example.com:587" {
		t.Errorf("expected addr smtp.example.com:587, got %q", gotAddr)
	}
	if gotFrom != "alerts@sitewatch.example" {
		t.Errorf("unexpected from: %q", gotFrom)
	}
	if len(gotTo) != 1 || gotTo[0] != "oncall@example.com" {
		t.Errorf("unexpected to: %v", gotTo)
	}
	if len(gotMsg) == 0 {
		t.Error("expected a non-empty message body")
	}
}

func TestEmailNotifier_Notify_PropagatesSendError(t *testing.T) {
	n := &EmailNotifier{
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
		From:     "a@example.com",
		To:       []string{"b@example.com"},
		send: func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
			return errFakeSMTP
		},
	}
	if err := n.Notify(context.Background(), fixedEvent(Down)); err == nil {
		t.Fatal("expected error to propagate from smtp send")
	}
}
