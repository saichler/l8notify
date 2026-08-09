package channel

import (
	"crypto/tls"
	"fmt"
	ntf "github.com/saichler/l8types/go/types/l8notify"
	"net/smtp"
	"strings"
	"time"
)

// SendEmail sends an email via SMTP.
func SendEmail(cfg *ntf.SmtpConfig, to, subject, body string) *ntf.DeliveryResult {
	if cfg == nil {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: "SMTP config is nil",
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}
	}

	if cfg.Host == "" || to == "" {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: "SMTP host or recipient is empty",
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}
	}

	fromAddr := cfg.FromAddress
	if fromAddr == "" {
		fromAddr = cfg.Username
	}

	fromDisplay := cfg.FromName
	if fromDisplay == "" {
		fromDisplay = fromAddr
	}

	msg := buildEmailMessage(fromDisplay, fromAddr, to, subject, body)
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	var err error
	if cfg.UseTls {
		err = sendTLS(addr, cfg.Host, auth, fromAddr, to, msg)
	} else {
		err = smtp.SendMail(addr, auth, fromAddr, []string{to}, msg)
	}

	if err != nil {
		return &ntf.DeliveryResult{
			Status:       ntf.DeliveryStatus_DELIVERY_STATUS_FAILED,
			ErrorMessage: fmt.Sprintf("email send failed: %v", err),
			Attempt:      1,
			SentAt:       time.Now().Unix(),
		}
	}

	return &ntf.DeliveryResult{
		Status:  ntf.DeliveryStatus_DELIVERY_STATUS_SENT,
		Attempt: 1,
		SentAt:  time.Now().Unix(),
	}
}

func buildEmailMessage(fromName, fromAddr, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s <%s>\r\n", fromName, fromAddr))
	b.WriteString(fmt.Sprintf("To: %s\r\n", to))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

func sendTLS(addr, host string, auth smtp.Auth, from, to string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: host}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("TLS dial failed: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client creation failed: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err = client.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}
	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("SMTP write failed: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("SMTP close failed: %w", err)
	}

	return client.Quit()
}
