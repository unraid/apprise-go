package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/xml"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// XMPPMessage is a message stanza the capture server accepted.
type XMPPMessage struct {
	To      string
	Type    string
	Subject string
	Body    string
}

// XMPPCapture is an XMPP server that negotiates just enough of a stream to
// receive message stanzas, so the two implementations can be run against the
// same endpoint and compared.
//
// XMPP is a stateful XML stream over a raw socket, which the HTTP capture
// harness cannot see. That left the port's wire format unchecked against
// upstream — the only provider in that position — so this stands in the same
// place smpp_capture.go does for SMPP.
//
// It negotiates STARTTLS with a certificate generated per run, because
// neither client will authenticate over a socket in the clear: mellium's SASL
// feature requires a secure session, and slixmpp defaults unencrypted_plain
// and unencrypted_scram to off. Both clients are pointed at it with verify=no,
// which is what makes a self-signed certificate workable.
type XMPPCapture struct {
	listener    net.Listener
	domain      string
	certificate tls.Certificate

	mu             sync.Mutex
	messages       []XMPPMessage
	rosterRequests int
	connections    int
	closed         bool
}

// StartXMPPCapture listens on a free port and serves until the test ends.
// domain is what the server calls itself in the stream header, which has to
// match the domain part of the client's JID.
func StartXMPPCapture(t *testing.T, domain string) *XMPPCapture {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for xmpp capture: %v", err)
	}

	certificate, err := selfSignedCertificate(domain)
	if err != nil {
		t.Fatalf("generate xmpp capture certificate: %v", err)
	}

	capture := &XMPPCapture{listener: listener, domain: domain, certificate: certificate}
	go capture.serve()

	t.Cleanup(func() {
		_ = capture.Close()
	})

	return capture
}

func (c *XMPPCapture) Addr() string {
	return c.listener.Addr().String()
}

func (c *XMPPCapture) Messages() []XMPPMessage {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]XMPPMessage(nil), c.messages...)
}

func (c *XMPPCapture) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = nil
	c.rosterRequests = 0
	c.connections = 0
}

func (c *XMPPCapture) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()

		return nil
	}
	c.closed = true
	c.mu.Unlock()

	return c.listener.Close()
}

func (c *XMPPCapture) serve() {
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			return
		}
		c.mu.Lock()
		c.connections++
		c.mu.Unlock()

		go c.handle(conn)
	}
}

// Connections reports how many times a client has connected. It is what
// separates a session held open across sends from one dialed per send —
// the stanzas are identical either way.
func (c *XMPPCapture) Connections() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.connections
}

// RosterRequests reports how many roster queries arrived.
func (c *XMPPCapture) RosterRequests() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.rosterRequests
}

func (c *XMPPCapture) recordRoster() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.rosterRequests++
}

func (c *XMPPCapture) record(message XMPPMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, message)
}

// WaitForMessages blocks until at least count messages have arrived, or the
// deadline passes. A client disconnects asynchronously after sending, so a
// test that read straight after Send would race it.
func (c *XMPPCapture) WaitForMessages(t *testing.T, count int, timeout time.Duration) []XMPPMessage {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		messages := c.Messages()
		if len(messages) >= count {
			return messages
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d xmpp messages, got %d before the deadline",
				count, len(messages))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

const (
	xmppTLSNamespace     = "urn:ietf:params:xml:ns:xmpp-tls"
	xmppSASLNamespace    = "urn:ietf:params:xml:ns:xmpp-sasl"
	xmppBindNamespace    = "urn:ietf:params:xml:ns:xmpp-bind"
	xmppSessionNamespace = "urn:ietf:params:xml:ns:xmpp-session"
)

func (c *XMPPCapture) handle(conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()

	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	if os.Getenv("APPRISE_XMPP_TRACE") != "" {
		conn = &tracedConn{Conn: conn}
	}

	// A stream restart after authentication starts the XML document over, so
	// each phase gets a decoder of its own; one carried across the restart
	// would see a second root element and fail.
	decoder := xml.NewDecoder(conn)
	if err := readStreamOpen(decoder); err != nil {
		return
	}

	// STARTTLS is offered alone: a client that sees mechanisms here would
	// have to decide whether to authenticate in the clear, and both refuse.
	if _, err := fmt.Fprint(conn, c.streamHeader()+
		`<stream:features>`+
		`<starttls xmlns='`+xmppTLSNamespace+`'><required/></starttls>`+
		`</stream:features>`); err != nil {
		return
	}

	element, err := nextStanza(decoder)
	if err != nil || element.Name.Local != "starttls" {
		return
	}
	if err := decoder.Skip(); err != nil {
		return
	}
	if _, err := fmt.Fprint(conn, `<proceed xmlns='`+xmppTLSNamespace+`'/>`); err != nil {
		return
	}

	secured := tls.Server(conn, &tls.Config{
		Certificates: []tls.Certificate{c.certificate},
		MinVersion:   tls.VersionTLS12,
	})
	if err := secured.Handshake(); err != nil {
		return
	}
	conn = secured
	if os.Getenv("APPRISE_XMPP_TRACE") != "" {
		conn = &tracedConn{Conn: conn}
	}

	// The stream starts over inside the encrypted socket.
	decoder = xml.NewDecoder(conn)
	if err := readStreamOpen(decoder); err != nil {
		return
	}
	if _, err := fmt.Fprint(conn, c.streamHeader()+
		`<stream:features>`+
		`<mechanisms xmlns='`+xmppSASLNamespace+`'>`+
		`<mechanism>PLAIN</mechanism>`+
		`</mechanisms>`+
		`</stream:features>`); err != nil {
		return
	}

	// Only the fact of authentication matters here, not the credential: both
	// clients are pointed at this server by a URL the test wrote.
	element, err = nextStanza(decoder)
	if err != nil || element.Name.Local != "auth" {
		return
	}
	if err := decoder.Skip(); err != nil {
		return
	}
	if _, err := fmt.Fprint(conn, `<success xmlns='`+xmppSASLNamespace+`'/>`); err != nil {
		return
	}

	decoder = xml.NewDecoder(conn)
	if err := readStreamOpen(decoder); err != nil {
		return
	}
	if _, err := fmt.Fprint(conn, c.streamHeader()+
		`<stream:features>`+
		`<bind xmlns='`+xmppBindNamespace+`'/>`+
		`<session xmlns='`+xmppSessionNamespace+`'/>`+
		`</stream:features>`); err != nil {
		return
	}

	c.serveStanzas(conn, decoder)
}

// tracedConn prints the stream in both directions, which is the only
// practical way to see where a negotiation stalls.
type tracedConn struct {
	net.Conn
}

func (t *tracedConn) Read(b []byte) (int, error) {
	n, err := t.Conn.Read(b)
	if n > 0 {
		fmt.Fprintf(os.Stderr, "C->S: %s\n", b[:n])
	}

	return n, err
}

func (t *tracedConn) Write(b []byte) (int, error) {
	fmt.Fprintf(os.Stderr, "S->C: %s\n", b)

	return t.Conn.Write(b)
}

func (c *XMPPCapture) streamHeader() string {
	return `<?xml version='1.0'?><stream:stream ` +
		`xmlns='jabber:client' xmlns:stream='http://etherx.jabber.org/streams' ` +
		`id='parity' from='` + c.domain + `' version='1.0'>`
}

type xmppIQ struct {
	ID   string `xml:"id,attr"`
	Type string `xml:"type,attr"`
	Bind *struct {
		Resource string `xml:"resource"`
	} `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
	Session *struct{} `xml:"urn:ietf:params:xml:ns:xmpp-session session"`
	Roster  *struct{} `xml:"jabber:iq:roster query"`
}

type xmppPresence struct {
	To   string    `xml:"to,attr"`
	From string    `xml:"from,attr"`
	Type string    `xml:"type,attr"`
	MUC  *struct{} `xml:"http://jabber.org/protocol/muc x"`
}

// xmppMessageStanza mirrors XMPPMessage field for field so a decoded stanza
// converts straight into one.
type xmppMessageStanza struct {
	To      string `xml:"to,attr"`
	Type    string `xml:"type,attr"`
	Subject string `xml:"subject"`
	Body    string `xml:"body"`
}

func (c *XMPPCapture) serveStanzas(conn net.Conn, decoder *xml.Decoder) {
	bound := "user@" + c.domain + "/apprise"

	for {
		element, err := nextStanza(decoder)
		if err != nil {
			return
		}

		switch element.Name.Local {
		case "iq":
			var iq xmppIQ
			if err := decoder.DecodeElement(&iq, &element); err != nil {
				return
			}

			switch {
			case iq.Roster != nil:
				// ?roster=yes asks for the contact list before sending. The
				// request is what the two implementations are compared on,
				// so it is recorded rather than only answered.
				c.recordRoster()
				_, err = fmt.Fprintf(conn,
					`<iq type='result' id='%s'><query xmlns='jabber:iq:roster'/></iq>`,
					iq.ID)

			case iq.Bind != nil:
				resource := iq.Bind.Resource
				if resource == "" {
					resource = "apprise"
				}
				bound = "user@" + c.domain + "/" + resource
				_, err = fmt.Fprintf(conn,
					`<iq type='result' id='%s'><bind xmlns='%s'><jid>%s</jid></bind></iq>`,
					iq.ID, xmppBindNamespace, bound)
			default:
				// Anything else — a session request, a roster query — is
				// answered empty. Nothing here depends on the contents.
				_, err = fmt.Fprintf(conn, `<iq type='result' id='%s'/>`, iq.ID)
			}
			if err != nil {
				return
			}

		case "message":
			var message xmppMessageStanza
			if err := decoder.DecodeElement(&message, &element); err != nil {
				return
			}
			c.record(XMPPMessage(message))

		case "presence":
			var presence xmppPresence
			if err := decoder.DecodeElement(&presence, &element); err != nil {
				return
			}

			// A presence addressed to a room with the MUC marker is a join
			// request. The client waits for its own presence back before it
			// will treat itself as in the room, so echo it with the
			// self-presence status code.
			if presence.MUC == nil || presence.To == "" {
				continue
			}
			if _, err := fmt.Fprintf(conn,
				`<presence from='%s' to='%s'>`+
					`<x xmlns='http://jabber.org/protocol/muc#user'>`+
					`<item affiliation='member' role='participant' jid='%s'/>`+
					`<status code='110'/>`+
					`</x></presence>`,
				presence.To, bound, bound); err != nil {
				return
			}

			// The room's subject is what ends the join sequence in XEP-0045.
			// Without it the client waits for the rest of a join that never
			// finishes and gives up before sending anything.
			room := presence.To
			if index := strings.IndexByte(room, '/'); index >= 0 {
				room = room[:index]
			}
			if _, err := fmt.Fprintf(conn,
				`<message from='%s' to='%s' type='groupchat'><subject></subject></message>`,
				room, bound); err != nil {
				return
			}

		default:
			// Anything else is read past and ignored.
			if err := decoder.Skip(); err != nil {
				return
			}
		}
	}
}

// readStreamOpen consumes the client's opening <stream:stream> tag, leaving
// the decoder positioned on its children.
func readStreamOpen(decoder *xml.Decoder) error {
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if start, ok := token.(xml.StartElement); ok {
			if start.Name.Local != "stream" {
				return fmt.Errorf("expected a stream open, got %s", start.Name.Local)
			}

			return nil
		}
	}
}

// nextStanza returns the next stanza's start element, skipping whitespace and
// stopping at the end of the stream.
func nextStanza(decoder *xml.Decoder) (xml.StartElement, error) {
	for {
		token, err := decoder.Token()
		if err != nil {
			return xml.StartElement{}, err
		}

		switch typed := token.(type) {
		case xml.StartElement:
			return typed, nil
		case xml.EndElement:
			if typed.Name.Local == "stream" {
				return xml.StartElement{}, io.EOF
			}
		case xml.CharData:
			if strings.TrimSpace(string(typed)) != "" {
				continue
			}
		}
	}
}

// selfSignedCertificate mints a certificate for the capture server. It is
// generated per run rather than committed so there is nothing to expire, and
// both clients are pointed at it with verify=no.
func selfSignedCertificate(domain string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: domain},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{domain, "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
