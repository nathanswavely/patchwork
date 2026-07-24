// Package mail is the single SMTP send path for the instance. Both magic
// links (internal/auth) and notification emails (internal/notifications)
// deliver through Send, so transport concerns — implicit TLS vs STARTTLS,
// which AUTH mechanism the server accepts — live here and nowhere else.
package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/config"
)

// Send delivers a full RFC 5322 message (headers included) to the given
// recipients using the configured SMTP relay. The transport mode is chosen
// by port: 465 and 2465 use implicit TLS (the connection is TLS from the
// first byte); every other port connects in plaintext and upgrades via
// STARTTLS when the server offers it.
func Send(cfg config.SMTP, to []string, msg []byte) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	return send(addr, implicitTLS(cfg.Port), cfg, to, msg, nil)
}

func implicitTLS(port int) bool {
	return port == 465 || port == 2465
}

// send is Send with the dial address and TLS config injectable so tests can
// point it at a local server with a self-signed certificate.
func send(addr string, implicit bool, cfg config.SMTP, to []string, msg []byte, tlsCfg *tls.Config) error {
	if tlsCfg == nil {
		tlsCfg = &tls.Config{ServerName: cfg.Host}
	}

	var client *smtp.Client
	if implicit {
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("smtp tls dial %s: %w", addr, err)
		}
		client, err = smtp.NewClient(conn, cfg.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("smtp handshake %s: %w", addr, err)
		}
	} else {
		var err error
		client, err = smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("smtp dial %s: %w", addr, err)
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				client.Close()
				return fmt.Errorf("smtp starttls: %w", err)
			}
		}
	}
	defer client.Close()

	if cfg.User != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(&fallbackAuth{user: cfg.User, pass: cfg.Pass}); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
	}

	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp rcpt %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close body: %w", err)
	}
	return client.Quit()
}

// fallbackAuth speaks AUTH PLAIN when the server advertises it and falls
// back to AUTH LOGIN otherwise (Office 365 and some appliance relays only
// offer LOGIN). Like the stdlib's PlainAuth, it refuses to send credentials
// over an unencrypted connection unless talking to localhost.
type fallbackAuth struct {
	user, pass string
	login      bool
}

func (a *fallbackAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !isLocalhost(server.Name) {
		return "", nil, errors.New("refusing to send credentials over unencrypted connection")
	}
	plain := false
	for _, mech := range server.Auth {
		if mech == "PLAIN" {
			plain = true
		}
	}
	if plain {
		return "PLAIN", []byte("\x00" + a.user + "\x00" + a.pass), nil
	}
	a.login = true
	return "LOGIN", nil, nil
}

func (a *fallbackAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	if !a.login {
		return nil, errors.New("unexpected server challenge during AUTH PLAIN")
	}
	switch prompt := strings.ToLower(strings.TrimSpace(string(fromServer))); {
	case strings.HasPrefix(prompt, "username"):
		return []byte(a.user), nil
	case strings.HasPrefix(prompt, "password"):
		return []byte(a.pass), nil
	default:
		return nil, fmt.Errorf("unexpected AUTH LOGIN challenge %q", fromServer)
	}
}

func isLocalhost(name string) bool {
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}
