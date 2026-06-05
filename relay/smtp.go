package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/mail"
	"strings"

	"github.com/emersion/go-smtp"
	"github.com/jhillyerd/enmime"
)

// Backend hands out a fresh Session per SMTP connection.
type Backend struct {
	cfg    Config
	client *Client
}

func (b *Backend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &Session{cfg: b.cfg, client: b.client}, nil
}

// Session accumulates the envelope (MAIL FROM / RCPT TO) and, on DATA, parses
// the MIME body and relays it to the worker.
type Session struct {
	cfg    Config
	client *Client
	from   string
	rcpts  []string
}

func (s *Session) Mail(from string, _ *smtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *Session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	if len(s.rcpts) == 0 {
		return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 5, 4}, Message: "no recipients"}
	}

	raw, err := io.ReadAll(io.LimitReader(r, s.cfg.MaxSize+1))
	if err != nil {
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "read failed"}
	}
	if int64(len(raw)) > s.cfg.MaxSize {
		return &smtp.SMTPError{Code: 552, EnhancedCode: smtp.EnhancedCode{5, 3, 4}, Message: "message too large"}
	}

	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	if err != nil {
		return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 6, 0}, Message: "parse failed: " + err.Error()}
	}

	payload := s.buildPayload(env)
	if err := s.client.Send(payload); err != nil {
		// 451 = transient; the sending MTA may retry.
		log.Printf("relay failed to=%v subject=%q: %v", payload.To, payload.Subject, err)
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "relay to worker failed"}
	}
	log.Printf("relayed to=%v from=%s subject=%q", payload.To, payload.From, payload.Subject)
	return nil
}

func (s *Session) Reset() {
	s.from = ""
	s.rcpts = nil
}

func (s *Session) Logout() error { return nil }

// buildPayload turns the SMTP envelope + parsed MIME into the worker's JSON
// request. Recipients come from RCPT TO (the real envelope), not the headers.
//
// If FROM_DOMAIN is set and the sender is off-domain, it's rewritten to
// DEFAULT_FROM with the original kept as Reply-To (the worker may restrict the
// From domain). If FROM_DOMAIN is empty, the original sender is passed through.
func (s *Session) buildPayload(env *enmime.Envelope) EmailPayload {
	p := EmailPayload{
		To:      append([]string(nil), s.rcpts...),
		Subject: env.GetHeader("Subject"),
		HTML:    env.HTML,
		Text:    env.Text,
	}

	origin := firstAddress(env.GetHeader("From"))
	if origin == "" {
		origin = s.from // envelope MAIL FROM
	}

	switch {
	case origin != "" && (s.cfg.FromDomain == "" || onDomain(origin, s.cfg.FromDomain)):
		p.From = origin
	case s.cfg.DefaultFrom != "":
		p.From = s.cfg.DefaultFrom
		if origin != "" {
			p.ReplyTo = origin
		}
	default:
		// No usable on-domain sender and no DEFAULT_FROM; pass the origin through
		// and let the worker decide (it may reject).
		p.From = origin
	}
	return p
}

// firstAddress extracts the bare address from a header value that may be
// "Name <addr@host>" or a comma-separated list.
func firstAddress(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	if addrs, err := mail.ParseAddressList(header); err == nil && len(addrs) > 0 {
		return addrs[0].Address
	}
	return header
}

func onDomain(addr, domain string) bool {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return false
	}
	return strings.EqualFold(addr[at+1:], domain)
}

func run(cfg Config) error {
	be := &Backend{cfg: cfg, client: NewClient(cfg)}
	addrs := splitListen(cfg.Listen)
	if len(addrs) == 0 {
		return fmt.Errorf("no listen address configured (SMTP_LISTEN)")
	}

	// One go-smtp server per address (e.g. loopback for host services plus a
	// docker-bridge IP for a containerized caller). They share the stateless
	// Backend. The function blocks until any listener exits.
	errc := make(chan error, len(addrs))
	for _, addr := range addrs {
		srv := smtp.NewServer(be)
		srv.Addr = addr
		srv.Domain = "localhost"
		srv.MaxMessageBytes = cfg.MaxSize
		srv.MaxRecipients = 50
		srv.AllowInsecureAuth = true // plaintext on trusted interfaces only (no STARTTLS)
		log.Printf("cf-email-relay listening on %s -> %s (from-domain=%q)", addr, cfg.WorkerURL, cfg.FromDomain)
		go func(s *smtp.Server) { errc <- s.ListenAndServe() }(srv)
	}
	return fmt.Errorf("smtp server: %w", <-errc)
}

// splitListen parses a comma-separated SMTP_LISTEN into trimmed addresses.
func splitListen(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
