package mailer

import (
	"fmt"
	"net/smtp"
)

// Mailer sends emails via SMTP (supports Gmail App Password auth).
type Mailer struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

// New creates a Mailer from the provided SMTP credentials.
func New(host, port, user, pass, from string) *Mailer {
	return &Mailer{Host: host, Port: port, User: user, Pass: pass, From: from}
}

// SendInvite sends an invitation email to toEmail with the given invite URL.
func (m *Mailer) SendInvite(toEmail, inviteURL string) error {
	auth := smtp.PlainAuth("", m.User, m.Pass, m.Host)
	subject := "You're invited to ZTNA"
	body := fmt.Sprintf(
		"Subject: %s\r\nFrom: %s\r\nTo: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n"+
			"You have been invited to join the ZTNA network.\r\n\r\n"+
			"Click the link below to accept your invitation (expires in 48 hours):\r\n\r\n%s\r\n\r\n"+
			"If you did not expect this invitation, please ignore this email.\r\n",
		subject, m.From, toEmail, inviteURL,
	)
	return smtp.SendMail(
		fmt.Sprintf("%s:%s", m.Host, m.Port),
		auth,
		m.From,
		[]string{toEmail},
		[]byte(body),
	)
}
