package notify

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	ircDefaultPort       = 6667
	ircDefaultSecurePort = 6697

	// Registration is the slowest step, since it waits for the server's
	// welcome; the others only wait for an echo.
	ircRegisterTimeout = 15 * time.Second
	ircJoinTimeout     = 6 * time.Second
	ircSendTimeout     = 4 * time.Second

	ircAuthNone     = "none"
	ircAuthServer   = "server"
	ircAuthNickServ = "nickserv"
	ircAuthZNC      = "znc"
)

// ircAuthModes fixes the order a mode prefix is matched in, which a map
// cannot. Upstream matches the first mode a prefix starts.
var ircAuthModes = []string{ircAuthNone, ircAuthServer, ircAuthNickServ, ircAuthZNC}

type IRCTarget struct {
	host     string
	port     int
	secure   bool
	verify   bool
	user     string
	password string

	nickname string
	fullname string
	authMode string
	join     bool

	// channels keeps its insertion order, since a channel's key travels with
	// it and the messages go out in the order the URL named them.
	channelOrder []string
	channelKeys  map[string]string
	users        []string
}

func NewIRCTarget(target *ParsedURL) (*IRCTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	secure := strings.EqualFold(target.Scheme, "ircs")

	port := target.Port
	if !target.HasPort || port == 0 {
		port = ircDefaultPort
		if secure {
			port = ircDefaultSecurePort
		}
	}

	// A mode is matched by prefix, so ?mode=nick reaches nickserv.
	authMode := ircAuthServer
	if raw := strings.ToLower(strings.TrimSpace(target.Query["mode"])); raw != "" {
		authMode = ""
		for _, candidate := range ircAuthModes {
			if strings.HasPrefix(candidate, raw) {
				authMode = candidate
				break
			}
		}
		if authMode == "" {
			return nil, fmt.Errorf("invalid auth mode: %s", target.Query["mode"])
		}
	}

	entries := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	channelOrder := []string{}
	channelKeys := map[string]string{}
	users := []string{}

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// A channel may carry a key after a comma: #room,secret.
		if strings.HasPrefix(entry, "#") || strings.HasPrefix(entry, "%23") {
			name, key := entry, ""
			if index := strings.IndexByte(entry, ','); index >= 0 {
				name, key = entry[:index], entry[index+1:]
			}
			name = ircNormalizeChannel(name)
			if name == "#" {
				continue
			}
			if _, seen := channelKeys[name]; !seen {
				channelOrder = append(channelOrder, name)
			}
			channelKeys[name] = key

			continue
		}

		users = append(users, strings.TrimPrefix(entry, "@"))
	}

	if len(channelOrder) == 0 && len(users) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	nickname := strings.TrimSpace(target.Query["nick"])
	if nickname == "" {
		nickname = strings.TrimSpace(target.User)
	}

	return &IRCTarget{
		host:         host,
		port:         port,
		secure:       secure,
		verify:       parseBoolWithDefault(target.Query["verify"], true),
		user:         strings.TrimSpace(target.User),
		password:     target.Password,
		nickname:     nickname,
		fullname:     strings.TrimSpace(target.Query["name"]),
		authMode:     authMode,
		join:         parseBoolWithDefault(target.Query["join"], true),
		channelOrder: channelOrder,
		channelKeys:  channelKeys,
		users:        users,
	}, nil
}

// ircNormalizeChannel gives a channel exactly one leading #.
func ircNormalizeChannel(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "%23")

	return "#" + strings.TrimLeft(name, "#")
}

// BuildRequest exists to satisfy Sender; IRC is a socket conversation, not a
// request.
func (i *IRCTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	_, _, _ = body, title, notifyType

	return RequestSpec{}, fmt.Errorf("irc is not a request-based protocol")
}

func (i *IRCTarget) Send(body, title string, notifyType NotifyType) error {
	_ = notifyType

	conn, err := i.dial()
	if err != nil {
		return err
	}

	client := &ircClient{
		conn:     conn,
		reader:   bufio.NewReader(conn),
		nickname: i.resolveNickname(),
	}
	defer func() {
		_ = conn.Close()
	}()

	if err := client.register(i); err != nil {
		return err
	}

	// Upstream declares no title support, so the framework folds the title
	// into the body with a CRLF before the plugin ever sees it, and the
	// plugin puts the result straight into the PRIVMSG.
	//
	// That newline ends the IRC line: a server reads everything after it as
	// a fresh command. This port reproduces it because matching upstream is
	// the contract, but it is upstream's defect and a command-injection
	// vector — a title or body carrying a newline can issue arbitrary IRC
	// commands as the sending user. See the next-steps doc.
	message := mergeTitleBody(title, body)

	for _, channel := range i.channelOrder {
		key := i.channelKeys[channel]
		if i.join || key != "" {
			if err := client.join(channel, key); err != nil {
				return err
			}
		}

		if err := client.privmsg(channel, message); err != nil {
			return err
		}
	}

	for _, user := range i.users {
		if err := client.privmsg(strings.TrimPrefix(user, "@"), message); err != nil {
			return err
		}
	}

	return client.quit()
}

func (i *IRCTarget) dial() (net.Conn, error) {
	address := net.JoinHostPort(i.host, strconv.Itoa(i.port))
	dialer := &net.Dialer{Timeout: ircRegisterTimeout}

	if !i.secure {
		conn, err := dialer.Dial("tcp", address)
		if err != nil {
			return nil, fmt.Errorf("irc connect to %s failed: %w", address, err)
		}

		return conn, nil
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		ServerName:         i.host,
		InsecureSkipVerify: !i.verify,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		return nil, fmt.Errorf("irc connect to %s failed: %w", address, err)
	}

	return conn, nil
}

// resolveNickname falls back to a generated nick the way upstream does when
// the URL names neither a nick nor a user.
func (i *IRCTarget) resolveNickname() string {
	if i.nickname != "" {
		return i.nickname
	}

	return "Apprise"
}

// registrationPassword is what travels in PASS. Only the server and bouncer
// modes send one, and a bouncer expects the user to be part of it.
func (i *IRCTarget) registrationPassword() string {
	switch i.authMode {
	case ircAuthZNC:
		return i.user + ":" + i.password
	case ircAuthServer:
		return i.password
	default:
		return ""
	}
}

// ircClient is one connection's worth of conversation.
type ircClient struct {
	conn     net.Conn
	reader   *bufio.Reader
	nickname string
}

func (c *ircClient) send(line string) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(ircSendTimeout))
	if _, err := fmt.Fprintf(c.conn, "%s\r\n", line); err != nil {
		return fmt.Errorf("irc write failed: %w", err)
	}

	return nil
}

// readLine returns the next line, answering server pings as it goes. A server
// that pings mid-registration expects an answer before it will continue.
func (c *ircClient) readLine(deadline time.Time) (ircMessage, error) {
	for {
		_ = c.conn.SetReadDeadline(deadline)
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return ircMessage{}, fmt.Errorf("irc read failed: %w", err)
		}

		message := parseIRCMessage(strings.TrimRight(line, "\r\n"))
		if message.command == "PING" {
			if err := c.send("PONG :" + message.trailing); err != nil {
				return ircMessage{}, err
			}

			continue
		}

		return message, nil
	}
}

// register sends the registration lines and waits for the server's welcome,
// renaming itself if the nick is taken.
func (c *ircClient) register(target *IRCTarget) error {
	if password := target.registrationPassword(); password != "" {
		if err := c.send("PASS " + password); err != nil {
			return err
		}
	}

	fullname := target.fullname
	if fullname == "" {
		fullname = "Apprise Notifications"
	}

	if err := c.send("NICK " + c.nickname); err != nil {
		return err
	}
	if err := c.send(fmt.Sprintf("USER %s 0 * :%s", c.nickname, fullname)); err != nil {
		return err
	}

	deadline := time.Now().Add(ircRegisterTimeout)
	for {
		message, err := c.readLine(deadline)
		if err != nil {
			return err
		}

		switch message.command {
		case "001":
			// Welcome: registration is complete.
			if target.authMode == ircAuthNickServ && target.password != "" {
				return c.send("PRIVMSG NickServ :IDENTIFY " + target.password)
			}

			return nil

		case "433", "436":
			// The nick is taken, so try another rather than giving up.
			c.nickname += "_"
			if err := c.send("NICK " + c.nickname); err != nil {
				return err
			}

		case "ERROR":
			return fmt.Errorf("irc registration refused: %s", message.trailing)
		}
	}
}

// join enters a channel and waits for the server to confirm it, since a
// message sent before the join lands is dropped.
func (c *ircClient) join(channel, key string) error {
	line := "JOIN " + channel
	if key != "" {
		line += " " + key
	}
	if err := c.send(line); err != nil {
		return err
	}

	deadline := time.Now().Add(ircJoinTimeout)
	for {
		message, err := c.readLine(deadline)
		if err != nil {
			return err
		}

		switch message.command {
		case "JOIN":
			// The echo carries the channel either as a parameter or as the
			// trailing argument, depending on the server.
			if strings.EqualFold(message.param(0), channel) ||
				strings.EqualFold(message.trailing, channel) {
				return nil
			}

		case "366":
			// End of names: the join is complete.
			if strings.EqualFold(message.param(1), channel) {
				return nil
			}

		case "403", "405", "471", "473", "474", "475":
			return fmt.Errorf("irc could not join %s: %s", channel, message.trailing)
		}
	}
}

// privmsg sends the message, one command per line.
//
// This is a deliberate divergence from upstream. Upstream declares
// title_maxlen = 0, so the framework folds the title into the body with a CRLF
// and the plugin writes the result straight into a single PRIVMSG. That
// newline ends the IRC line: a server reads everything after it as a fresh
// command, so a title or body carrying one can issue arbitrary IRC commands as
// the sending user. Notification bodies routinely carry text this port did not
// author — that is what alerting is — which makes it reachable.
//
// Matching upstream byte for byte would reproduce the injection, and what it
// reproduces is malformed output rather than a delivered message: upstream is
// not trying to send two commands. Splitting per line is what an IRC client is
// supposed to do, keeps the visible result the same, and closes the hole.
func (c *ircClient) privmsg(target, message string) error {
	for _, line := range ircMessageLines(message) {
		if err := c.privmsgLine(target, line); err != nil {
			return err
		}
	}

	return nil
}

// ircMessageLines splits on any newline and drops blank lines, since an empty
// PRIVMSG is rejected. A message with no newline yields itself, so the ordinary
// case is one command exactly as before.
func ircMessageLines(message string) []string {
	lines := []string{}
	for _, line := range strings.FieldsFunc(message, func(r rune) bool {
		return r == '\r' || r == '\n'
	}) {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		return []string{""}
	}

	return lines
}

func (c *ircClient) privmsgLine(target, message string) error {
	return c.send(fmt.Sprintf("PRIVMSG %s :%s", target, message))
}

func (c *ircClient) quit() error {
	return c.send("QUIT :Apprise Notifications")
}

// ircMessage is one parsed line: [":" prefix] command params [":" trailing].
type ircMessage struct {
	prefix   string
	command  string
	params   []string
	trailing string
}

func (m ircMessage) param(index int) string {
	if index < 0 || index >= len(m.params) {
		return ""
	}

	return m.params[index]
}

func parseIRCMessage(line string) ircMessage {
	message := ircMessage{}

	if strings.HasPrefix(line, ":") {
		prefix, rest, found := strings.Cut(line[1:], " ")
		message.prefix = prefix
		if !found {
			return message
		}
		line = rest
	}

	// The trailing argument starts at the first " :" and runs to the end, so
	// it is split off before the parameters are counted.
	if head, tail, found := strings.Cut(line, " :"); found {
		message.trailing = tail
		line = head
	}

	fields := strings.Fields(line)
	if len(fields) > 0 {
		message.command = strings.ToUpper(fields[0])
		message.params = fields[1:]
	}

	return message
}

func init() {
	RegisterSchemaEntryOrdered(166, SchemaEntry{
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
				"join": map[string]any{
					"default":  true,
					"map_to":   "join",
					"name":     "Join Channels",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"mode": map[string]any{
					"default":  "server",
					"map_to":   "mode",
					"name":     "Auth Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"znc", "server", "nickserv", "none"},
				},
				"name": map[string]any{
					"map_to":   "name",
					"name":     "Real Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"nick": map[string]any{
					"map_to":   "nick",
					"name":     "Nickname",
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
				"rto": map[string]any{
					"default":  4.0,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{host}/{targets}", "{schema}://{host}:{port}/{targets}", "{schema}://{user}@{host}/{targets}", "{schema}://{user}@{host}:{port}/{targets}", "{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{targets}"},
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
					"required": false,
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
					"values":   []string{"irc", "ircs"},
				},
				"target_channel": map[string]any{
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
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_channel", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "User",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"irc"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"ircs"},
		"service_name":     "IRC",
		"service_url":      "https://ircv3.net/",
		"setup_url":        "https://appriseit.com/services/irc/",
	})
}
