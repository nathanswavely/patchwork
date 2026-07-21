package mail

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/config"
)

// received records what the test server saw during one delivery.
type received struct {
	authMech string // "PLAIN" or "LOGIN"
	authUser string
	authPass string
	from     string
	rcpts    []string
	data     string
	overTLS  bool // the DATA phase happened on a TLS connection
}

type testServer struct {
	addr     string
	implicit bool   // TLS from the first byte (vs plaintext + STARTTLS)
	mechs    string // what EHLO advertises, e.g. "PLAIN" or "LOGIN"
	tlsConf  *tls.Config
	got      chan received
}

func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "smtp test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func startServer(t *testing.T, implicit bool, mechs string) (*testServer, *x509.CertPool) {
	t.Helper()
	cert, pool := selfSigned(t)
	srv := &testServer{
		implicit: implicit,
		mechs:    mechs,
		tlsConf:  &tls.Config{Certificates: []tls.Certificate{cert}},
		got:      make(chan received, 1),
	}
	var ln net.Listener
	var err error
	if implicit {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", srv.tlsConf)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	srv.addr = ln.Addr().String()
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		srv.serve(t, conn)
	}()
	return srv, pool
}

func (s *testServer) serve(t *testing.T, conn net.Conn) {
	defer conn.Close()
	rec := received{overTLS: s.implicit}
	r := bufio.NewReader(conn)
	write := func(line string) { conn.Write([]byte(line + "\r\n")) }
	readLine := func() string {
		line, err := r.ReadString('\n')
		if err != nil {
			return ""
		}
		return strings.TrimRight(line, "\r\n")
	}
	b64 := base64.StdEncoding

	write("220 test ESMTP")
	for {
		line := readLine()
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			write("250-test greets you")
			write("250-AUTH " + s.mechs)
			if !s.implicit && !rec.overTLS {
				write("250-STARTTLS")
			}
			write("250 8BITMIME")
		case cmd == "STARTTLS":
			write("220 ready for TLS")
			tconn := tls.Server(conn, s.tlsConf)
			if err := tconn.Handshake(); err != nil {
				t.Errorf("server-side TLS handshake: %v", err)
				return
			}
			conn = tconn
			r = bufio.NewReader(conn)
			rec.overTLS = true
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			rec.authMech = "PLAIN"
			raw, _ := b64.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN"):]))
			parts := strings.Split(string(raw), "\x00")
			if len(parts) == 3 {
				rec.authUser, rec.authPass = parts[1], parts[2]
			}
			write("235 ok")
		case strings.HasPrefix(cmd, "AUTH LOGIN"):
			rec.authMech = "LOGIN"
			write("334 " + b64.EncodeToString([]byte("Username:")))
			u, _ := b64.DecodeString(readLine())
			rec.authUser = string(u)
			write("334 " + b64.EncodeToString([]byte("Password:")))
			p, _ := b64.DecodeString(readLine())
			rec.authPass = string(p)
			write("235 ok")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			rec.from = angleAddr(line)
			write("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			rec.rcpts = append(rec.rcpts, angleAddr(line))
			write("250 ok")
		case cmd == "DATA":
			write("354 go ahead")
			var body strings.Builder
			for {
				dl := readLine()
				if dl == "." {
					break
				}
				body.WriteString(dl + "\n")
			}
			rec.data = body.String()
			write("250 queued")
		case cmd == "QUIT":
			write("221 bye")
			s.got <- rec
			return
		default:
			write("500 unrecognized")
		}
	}
}

// angleAddr pulls the address out of "MAIL FROM:<a@b> BODY=8BITMIME" etc.
func angleAddr(line string) string {
	open := strings.Index(line, "<")
	close := strings.Index(line, ">")
	if open < 0 || close < open {
		return line
	}
	return line[open+1 : close]
}

func deliver(t *testing.T, srv *testServer, pool *x509.CertPool) received {
	t.Helper()
	cfg := config.SMTP{
		Host: "127.0.0.1",
		User: "sender@example.org",
		Pass: "hunter2",
		From: "patchwork@example.org",
	}
	tlsCfg := &tls.Config{ServerName: "127.0.0.1", RootCAs: pool}
	msg := []byte("Subject: hello\r\n\r\nbody line\r\n")
	if err := send(srv.addr, srv.implicit, cfg, []string{"member@example.org"}, msg, tlsCfg); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case rec := <-srv.got:
		return rec
	case <-time.After(5 * time.Second):
		t.Fatal("server never saw a complete delivery")
		return received{}
	}
}

func checkEnvelope(t *testing.T, rec received) {
	t.Helper()
	if rec.from != "patchwork@example.org" {
		t.Errorf("MAIL FROM = %q", rec.from)
	}
	if len(rec.rcpts) != 1 || rec.rcpts[0] != "member@example.org" {
		t.Errorf("RCPT TO = %v", rec.rcpts)
	}
	if !strings.Contains(rec.data, "body line") {
		t.Errorf("body not delivered, got %q", rec.data)
	}
	if rec.authUser != "sender@example.org" || rec.authPass != "hunter2" {
		t.Errorf("credentials = %q / %q", rec.authUser, rec.authPass)
	}
	if !rec.overTLS {
		t.Error("delivery was not over TLS")
	}
}

func TestSendSTARTTLSPlain(t *testing.T) {
	srv, pool := startServer(t, false, "PLAIN LOGIN")
	rec := deliver(t, srv, pool)
	checkEnvelope(t, rec)
	if rec.authMech != "PLAIN" {
		t.Errorf("auth mech = %q, want PLAIN", rec.authMech)
	}
}

func TestSendSTARTTLSLoginFallback(t *testing.T) {
	srv, pool := startServer(t, false, "LOGIN")
	rec := deliver(t, srv, pool)
	checkEnvelope(t, rec)
	if rec.authMech != "LOGIN" {
		t.Errorf("auth mech = %q, want LOGIN", rec.authMech)
	}
}

func TestSendImplicitTLS(t *testing.T) {
	srv, pool := startServer(t, true, "PLAIN")
	rec := deliver(t, srv, pool)
	checkEnvelope(t, rec)
	if rec.authMech != "PLAIN" {
		t.Errorf("auth mech = %q, want PLAIN", rec.authMech)
	}
}

func TestSendImplicitTLSLoginFallback(t *testing.T) {
	srv, pool := startServer(t, true, "LOGIN")
	rec := deliver(t, srv, pool)
	checkEnvelope(t, rec)
	if rec.authMech != "LOGIN" {
		t.Errorf("auth mech = %q, want LOGIN", rec.authMech)
	}
}

func TestImplicitTLSPortSelection(t *testing.T) {
	for port, want := range map[int]bool{465: true, 2465: true, 587: false, 25: false, 1025: false} {
		if got := implicitTLS(port); got != want {
			t.Errorf("implicitTLS(%d) = %v, want %v", port, got, want)
		}
	}
}
