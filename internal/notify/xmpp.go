package notify

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mellium.im/sasl"
	"mellium.im/xmlstream"
	"mellium.im/xmpp"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/stanza"
)

// XMPP is the one service here that speaks a stateful protocol over a raw
// socket rather than HTTP, so it carries a dependency — mellium.im/xmpp — for
// stream negotiation, STARTTLS and SASL. Everything else in this port is
// pure standard library.

const (
	xmppModeNone     = "none"
	xmppModeTLS      = "tls"
	xmppModeStartTLS = "starttls"
)

// Each secure mode has its own conventional port.
var xmppModePorts = map[string]int{
	xmppModeNone:     5222,
	xmppModeTLS:      5223,
	xmppModeStartTLS: 5222,
}

// A JID is [#]local[@domain][/resource]; a leading # marks a multi-user chat.
var xmppJIDPattern = regexp.MustCompile(`^(#)?([^@\s/]+)(?:@([^@\s/]+))?(?:/([^/\s]+))?$`)

type xmppRecipient struct {
	address string
	groupChat
}

type groupChat = struct{ isMUC bool }

type XMPPTarget struct {
	jid        string
	password   string
	host       string
	port       int
	mode       string
	verify     bool
	useSubject bool
	nickname   string
	timeout    time.Duration
	targets    []xmppRecipient
}

func NewXMPPTarget(target *ParsedURL) (*XMPPTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode == "" {
		mode = xmppModeStartTLS
	}
	if _, ok := xmppModePorts[mode]; !ok {
		return nil, fmt.Errorf("invalid secure mode: %s", mode)
	}

	// The connection host may differ from the JID's domain, the same way the
	// email plugin separates smtp= from the address.
	connectHost := host
	if override := strings.TrimSpace(target.Query["xmpp"]); override != "" {
		connectHost = override
	}

	userJID, _, err := normalizeXMPPJID(target.User, host)
	if err != nil {
		return nil, fmt.Errorf("invalid jid: %s", target.User)
	}

	entries := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	targets := make([]xmppRecipient, 0, len(entries))
	for _, entry := range sortedUniqueTargets(entries) {
		address, isMUC, err := normalizeXMPPJID(entry, host)
		if err != nil {
			continue
		}
		targets = append(targets, xmppRecipient{address: address, groupChat: groupChat{isMUC: isMUC}})
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	port := target.Port
	if port == 0 {
		port = xmppModePorts[mode]
	}

	return &XMPPTarget{
		jid:        userJID,
		password:   target.Password,
		host:       connectHost,
		port:       port,
		mode:       mode,
		verify:     parseBoolWithDefault(target.Query["verify"], true),
		useSubject: parseBoolWithDefault(target.Query["subject"], false),
		nickname:   strings.TrimSpace(target.Query["name"]),
		timeout:    15 * time.Second,
		targets:    targets,
	}, nil
}

// BuildRequest cannot describe this provider: XMPP is a stateful XML stream,
// not a request.
func (x *XMPPTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	_, _, _ = body, title, notifyType

	return RequestSpec{}, fmt.Errorf("xmpp is not a request-based protocol")
}

func (x *XMPPTarget) Send(body, title string, notifyType NotifyType) error {
	_ = notifyType

	ctx, cancel := context.WithTimeout(context.Background(), x.timeout)
	defer cancel()

	session, err := x.dial(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = session.Close()
	}()

	// A subject is only sent when asked for; otherwise the title is folded
	// into the body, the way every other notifier here handles it.
	subject := ""
	message := body
	if x.useSubject {
		subject = title
	} else {
		message = mergeTitleBody(title, body)
	}

	for _, recipient := range x.targets {
		to, err := jid.Parse(recipient.address)
		if err != nil {
			continue
		}

		messageType := stanza.ChatMessage
		if recipient.isMUC {
			messageType = stanza.GroupChatMessage
		}

		if err := session.Encode(ctx, xmppMessage{
			Message: stanza.Message{To: to, Type: messageType},
			Subject: subject,
			Body:    message,
		}); err != nil {
			return fmt.Errorf("xmpp send to %s failed: %w", recipient.address, err)
		}
	}

	return nil
}

// xmppMessage is a message stanza with the subject and body children; the
// library models the envelope but leaves the payload to the caller.
type xmppMessage struct {
	stanza.Message

	XMLName xml.Name `xml:"jabber:client message"`
	Subject string   `xml:"subject,omitempty"`
	Body    string   `xml:"body"`
}

func (x *XMPPTarget) dial(ctx context.Context) (*xmpp.Session, error) {
	parsedJID, err := jid.Parse(x.jid)
	if err != nil {
		return nil, fmt.Errorf("invalid jid %s: %w", x.jid, err)
	}

	address := net.JoinHostPort(x.host, strconv.Itoa(x.port))
	tlsConfig := &tls.Config{
		ServerName:         parsedJID.Domain().String(),
		InsecureSkipVerify: !x.verify,
		MinVersion:         tls.VersionTLS12,
	}

	var conn net.Conn
	dialer := &net.Dialer{Timeout: x.timeout}
	if x.mode == xmppModeTLS {
		// Direct TLS wraps the socket before any XML is exchanged.
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, fmt.Errorf("xmpp connect to %s failed: %w", address, err)
	}

	features := []xmpp.StreamFeature{
		xmpp.BindResource(),
		// SCRAM before PLAIN so a password is never sent in the clear when
		// the server offers anything better.
		xmpp.SASL("", x.password, sasl.ScramSha256, sasl.ScramSha1, sasl.Plain),
	}
	if x.mode == xmppModeStartTLS {
		features = append([]xmpp.StreamFeature{xmpp.StartTLS(tlsConfig)}, features...)
	}

	session, err := xmpp.NewClientSession(ctx, parsedJID, conn, features...)
	if err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("xmpp session for %s failed: %w", x.jid, err)
	}

	// The stream has to be readable for the server to make progress, but
	// nothing here acts on what arrives.
	go func() {
		_ = session.Serve(xmpp.HandlerFunc(func(_ xmlstream.TokenReadEncoder, _ *xml.StartElement) error {
			return nil
		}))
	}()

	return session, nil
}

// normalizeXMPPJID fills in the default domain and reports whether the entry
// named a multi-user chat.
func normalizeXMPPJID(value, defaultHost string) (string, bool, error) {
	raw := strings.TrimSpace(value)
	matches := xmppJIDPattern.FindStringSubmatch(raw)
	if matches == nil {
		return "", false, fmt.Errorf("invalid jid")
	}

	isMUC := matches[1] == "#"
	local, domain, resource := matches[2], matches[3], matches[4]
	if domain == "" {
		domain = defaultHost
	}
	if domain == "" {
		return "", false, fmt.Errorf("invalid jid")
	}

	address := local + "@" + domain
	if resource != "" {
		address += "/" + resource
	}

	return address, isMUC, nil
}
func init() {
	RegisterSchemaEntryOrdered(165, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"cto": map[string]any{
					"default":  4.0,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"emojis": map[string]any{
					"default":  false,
					"map_to":   "emojis",
					"name":     "Interpret Emojis",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"format": map[string]any{
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"keepalive": map[string]any{
					"default":  false,
					"map_to":   "keepalive",
					"name":     "Keep Connection Alive",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"mode": map[string]any{
					"default":  "starttls",
					"map_to":   "secure_mode",
					"name":     "Secure Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"starttls", "tls", "none"},
				},
				"name": map[string]any{
					"map_to":   "name",
					"name":     "MUC Nickname",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"optional": map[string]any{
					"default":  false,
					"map_to":   "optional",
					"name":     "Optional Service",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"overflow": map[string]any{
					"default":  "upstream",
					"map_to":   "overflow",
					"name":     "Overflow Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"split", "truncate", "upstream"},
				},
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"retry": map[string]any{
					"default":  0,
					"map_to":   "retry",
					"max":      10,
					"min":      0,
					"name":     "Service Retry",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"roster": map[string]any{
					"default":  false,
					"map_to":   "roster",
					"name":     "Get Roster",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"rto": map[string]any{
					"default":  4.0,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"scramplus": map[string]any{
					"default":  true,
					"map_to":   "scramplus",
					"name":     "SCRAM-PLUS Channel Binding",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"subject": map[string]any{
					"default":  false,
					"map_to":   "subject",
					"name":     "Use Subject",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"verify": map[string]any{
					"default":  true,
					"map_to":   "verify",
					"name":     "Verify SSL",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"wait": map[string]any{
					"default":  0.0,
					"map_to":   "wait",
					"max":      20.0,
					"min":      0.0,
					"name":     "Inter-Retry Wait",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"xmpp": map[string]any{
					"map_to":   "xmpp_host",
					"name":     "XMPP Server",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{user}:{password}@{host}", "{schema}://{user}:{password}@{host}:{port}", "{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"port": map[string]any{
					"map_to":   "port",
					"max":      65535,
					"min":      1,
					"name":     "Port",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"xmpp", "xmpps"},
				},
				"target_channels": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_channels", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "User",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"xmpp"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{"slixmpp"},
		},
		"secure_protocols": []string{"xmpps"},
		"service_name":     "XMPP",
		"service_url":      "https://xmpp.org/",
		"setup_url":        "https://appriseit.com/services/xmpp/",
	})
}
