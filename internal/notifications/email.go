package notifications

import (
	"fmt"
	"log"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/mail"
)

// EmailChannel sends notifications via SMTP. Only available when SMTP is configured.
type EmailChannel struct {
	SMTP         *config.SMTP
	Domain       string
	InstanceName string
}

func (c *EmailChannel) Name() string { return "email" }

func (c *EmailChannel) Available() bool {
	return c.SMTP != nil && c.SMTP.Configured()
}

func (c *EmailChannel) Send(db *database.DB, recipientID string, event Event) {
	if !c.Available() {
		return
	}

	// Look up recipient email.
	var email string
	if err := db.QueryRow("SELECT email FROM users WHERE id = ?", recipientID).Scan(&email); err != nil || email == "" {
		return
	}

	go c.sendEmail(email, event)
}

func (c *EmailChannel) sendEmail(to string, event Event) {
	subject := event.Title
	if event.NodeName != "" {
		subject = fmt.Sprintf("[%s] %s", event.NodeName, event.Title)
	}

	linkURL := ""
	if event.Link != "" {
		linkURL = fmt.Sprintf("https://%s%s", c.Domain, event.Link)
	}

	prefsURL := fmt.Sprintf("https://%s/settings/notifications", c.Domain)

	body := buildEmailBody(emailData{
		Subject:      subject,
		Title:        event.Title,
		Body:         event.Body,
		LinkURL:      linkURL,
		PatchName:    event.NodeName,
		InstanceName: c.InstanceName,
		PrefsURL:     prefsURL,
	})

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/html; charset=UTF-8\r\nMIME-Version: 1.0\r\n\r\n%s",
		c.SMTP.From, to, subject, body,
	)

	if err := mail.Send(*c.SMTP, []string{to}, []byte(msg)); err != nil {
		log.Printf("notifications: email to %s failed: %v", to, err)
	}
}

type emailData struct {
	Subject      string
	Title        string
	Body         string
	LinkURL      string
	PatchName    string
	InstanceName string
	PrefsURL     string
}

func buildEmailBody(d emailData) string {
	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html><html><body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 560px; margin: 0 auto; padding: 20px;">`)

	if d.PatchName != "" {
		sb.WriteString(fmt.Sprintf(`<p style="color: #666; font-size: 13px; margin: 0 0 4px;">%s</p>`, escapeHTML(d.PatchName)))
	}

	sb.WriteString(fmt.Sprintf(`<h2 style="margin: 0 0 12px; font-size: 18px; color: #1a1a1a;">%s</h2>`, escapeHTML(d.Title)))

	if d.Body != "" {
		sb.WriteString(fmt.Sprintf(`<p style="color: #444; font-size: 14px; line-height: 1.5; margin: 0 0 16px;">%s</p>`, escapeHTML(d.Body)))
	}

	if d.LinkURL != "" {
		sb.WriteString(fmt.Sprintf(
			`<p><a href="%s" style="display: inline-block; padding: 10px 20px; background: #5B21B6; color: #fff; text-decoration: none; border-radius: 4px; font-size: 14px;">View in Patchwork</a></p>`,
			d.LinkURL,
		))
	}

	sb.WriteString(`<hr style="border: none; border-top: 1px solid #eee; margin: 24px 0;">`)
	sb.WriteString(fmt.Sprintf(
		`<p style="font-size: 12px; color: #999;">You received this because you're a member of %s on %s. <a href="%s" style="color: #999;">Manage notification preferences</a></p>`,
		escapeHTML(d.PatchName), escapeHTML(d.InstanceName), d.PrefsURL,
	))

	sb.WriteString(`</body></html>`)
	return sb.String()
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
