package alert

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// smtpSendFunc matches net/smtp.SendMail's signature. Production code uses
// smtp.SendMail directly; tests inject a fake to capture the message
// without spinning up a real SMTP server.
type smtpSendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error

// EmailNotifier sends alerts as plain-text email via SMTP with AUTH PLAIN.
type EmailNotifier struct {
	SMTPHost string
	SMTPPort int
	Username string
	Password string
	From     string
	To       []string

	// send defaults to smtp.SendMail; overridden in tests.
	send smtpSendFunc
}

// Name implements Notifier.
func (n *EmailNotifier) Name() string { return "email" }

func (n *EmailNotifier) sendFunc() smtpSendFunc {
	if n.send != nil {
		return n.send
	}
	return smtp.SendMail
}

// Notify implements Notifier. ctx is accepted for interface symmetry with
// the other notifiers; net/smtp has no context-aware API, so send happens
// synchronously.
func (n *EmailNotifier) Notify(_ context.Context, ev Event) error {
	msg := BuildEmailMessage(n.From, n.To, ev)

	var auth smtp.Auth
	if n.Username != "" {
		auth = smtp.PlainAuth("", n.Username, n.Password, n.SMTPHost)
	}

	addr := fmt.Sprintf("%s:%d", n.SMTPHost, n.SMTPPort)
	if err := n.sendFunc()(addr, auth, n.From, n.To, []byte(msg)); err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	return nil
}

// BuildEmailMessage renders a minimal RFC 5322 message: From/To/Subject
// headers, a blank line, then a plain-text body. It is exported so golden
// tests can assert on the exact bytes without sending mail.
func BuildEmailMessage(from string, to []string, ev Event) string {
	subject := fmt.Sprintf("[sitewatch] %s: %s", ev.Kind.String(), ev.CheckName)

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(summary(ev))
	b.WriteString("\r\n")
	if d := detail(ev); d != "" {
		b.WriteString("\r\n")
		b.WriteString(d)
		b.WriteString("\r\n")
	}
	return b.String()
}
