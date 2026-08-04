package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// A #-prefixed target is a channel name; anything else is a channel ID.
var (
	mattermostChannelPattern   = regexp.MustCompile(`^#(?P<name>[A-Za-z0-9_-]+)$`)
	mattermostChannelIDPattern = regexp.MustCompile(`^\+?(?P<name>[A-Za-z0-9_-]+)$`)
)

// mattermostTarget carries the kind alongside the value, because a channel
// name needs a lookup in bot mode while an ID does not.
type mattermostTarget struct {
	byName bool
	value  string
}

type MattermostTarget struct {
	host         string
	port         int
	secure       bool
	fullPath     string
	token        string
	username     string
	includeImage bool
	iconURL      string
	mode         string
	channels     []mattermostTarget
}

func NewMattermostTarget(target *ParsedURL) (*MattermostTarget, error) {
	if strings.TrimSpace(target.Host) == "" {
		return nil, fmt.Errorf("missing host")
	}

	segments := splitPath(target.Path)
	if len(segments) == 0 {
		return nil, fmt.Errorf("missing token")
	}
	token := segments[len(segments)-1]
	fullPath := ""
	if len(segments) > 1 {
		fullPath = "/" + strings.Join(segments[:len(segments)-1], "/")
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode == "" {
		mode = "webhook"
	}
	matched := ""
	for _, candidate := range []string{"webhook", "bot"} {
		if strings.HasPrefix(candidate, mode) {
			matched = candidate
			break
		}
	}
	if matched == "" {
		return nil, fmt.Errorf("invalid mode: %s", target.Query["mode"])
	}
	mode = matched

	// The team comes from the user field, under either of its two names.
	username := strings.TrimSpace(target.User)
	if botname := strings.TrimSpace(target.Query["botname"]); botname != "" {
		username = botname
	}
	if team := strings.TrimSpace(target.Query["team"]); team != "" {
		username = team
	}

	entries := []string{}
	for _, key := range []string{"channels", "channel", "to"} {
		if value, ok := target.Query[key]; ok && strings.TrimSpace(value) != "" {
			entries = append(entries, parseDelimitedList(value)...)
		}
	}

	channels := []mattermostTarget{}
	for _, entry := range sortedUniqueTargets(entries) {
		if match := mattermostChannelPattern.FindStringSubmatch(entry); match != nil {
			// Resolving a name to an ID needs a team, so bot mode drops it
			// rather than issuing a lookup that cannot succeed.
			if mode == "bot" && username == "" {
				continue
			}
			channels = append(channels, mattermostTarget{byName: true, value: match[1]})
			continue
		}
		if match := mattermostChannelIDPattern.FindStringSubmatch(entry); match != nil {
			// A bare token is a channel name to a webhook and a channel ID to
			// the API.
			channels = append(channels, mattermostTarget{byName: mode == "webhook", value: match[1]})
		}
	}

	if mode == "bot" && len(channels) == 0 {
		return nil, fmt.Errorf("missing channels")
	}

	return &MattermostTarget{
		host:         target.Host,
		port:         target.Port,
		secure:       target.Scheme == "mmosts",
		fullPath:     fullPath,
		token:        token,
		username:     username,
		includeImage: parseBool(target.Query["image"], true),
		iconURL:      strings.TrimSpace(target.Query["icon_url"]),
		mode:         mode,
		channels:     channels,
	}, nil
}

func (m *MattermostTarget) Send(body, title string, notifyType NotifyType) error {
	return m.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments uploads each file against the channel it is going to,
// then references the returned ids in the post. Only bot mode can upload;
// a webhook has no file API.
func (m *MattermostTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	message := mergeTitleBody(title, body)

	// A webhook with no channel posts to whichever one it is bound to.
	channels := m.channels
	if len(channels) == 0 {
		channels = []mattermostTarget{{}}
	}

	for _, channel := range channels {
		if m.mode == "bot" && channel.byName {
			resolved, err := m.resolveChannelID(channel.value)
			if err != nil {
				return err
			}
			channel = mattermostTarget{value: resolved}
		}

		fileIDs := []string{}
		if m.mode == "bot" {
			for index, attachment := range attachments {
				id, err := m.uploadFile(channel.value, attachment, index)
				if err != nil {
					return err
				}
				fileIDs = append(fileIDs, id)
			}
		}

		spec, err := m.buildSpec(message, notifyType, channel, fileIDs)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

// uploadFile posts one file to the channel and returns the id the post
// references.
func (m *MattermostTarget) uploadFile(channelID string, attachment Attachment, index int) (string, error) {
	fields := formFields{}
	fields.Set("channel_id", channelID)

	requestBody, contentType, err := singleFileAttachmentBody(
		fields, "files",
		Attachment{
			Name:     attachment.FileName(index, ".dat"),
			MimeType: attachment.MimeType,
			Data:     attachment.Data,
		}, true)
	if err != nil {
		return "", err
	}

	var response struct {
		FileInfos []struct {
			ID string `json:"id"`
		} `json:"file_infos"`
	}
	if err := doJSONRequest(RequestSpec{
		Method: "POST",
		URL:    m.baseURL() + "/api/v4/files",
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Authorization": "Bearer " + m.token,
			"Content-Type":  contentType,
		},
		Body: requestBody,
	}, &response); err != nil {
		return "", err
	}
	if len(response.FileInfos) == 0 || response.FileInfos[0].ID == "" {
		return "", fmt.Errorf("mattermost file upload returned no id")
	}

	return response.FileInfos[0].ID, nil
}

func (m *MattermostTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)
	channel := mattermostTarget{}
	if len(m.channels) > 0 {
		channel = m.channels[0]
	}

	return m.buildSpec(message, notifyType, channel, nil)
}

// resolveChannelID turns a channel name into the ID the API posts to, which
// is why bot mode needs a team.
func (m *MattermostTarget) resolveChannelID(name string) (string, error) {
	spec := RequestSpec{
		Method: "GET",
		URL: fmt.Sprintf("%s/api/v4/teams/name/%s/channels/name/%s",
			m.baseURL(), m.username, name),
		// A lookup carries no body, so upstream sets no content type and
		// asks for JSON back.
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "application/json",
			"Authorization": "Bearer " + m.token,
		},
	}

	var response struct {
		ID string `json:"id"`
	}
	if err := doJSONRequest(spec, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ID) == "" {
		return "", fmt.Errorf("could not resolve channel %s", name)
	}

	return response.ID, nil
}

func (m *MattermostTarget) baseURL() string {
	scheme := "http"
	if m.secure {
		scheme = "https"
	}
	host := m.host
	if m.port != 0 {
		host = fmt.Sprintf("%s:%d", host, m.port)
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, strings.TrimRight(m.fullPath, "/"))
}

func (m *MattermostTarget) buildSpec(message string, notifyType NotifyType, channel mattermostTarget, fileIDs []string) (RequestSpec, error) {
	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}

	var payload map[string]any
	var url string

	if m.mode == "bot" {
		// The API posts by channel ID and carries none of the webhook's
		// presentation fields.
		payload = map[string]any{
			"channel_id": channel.value,
			"message":    message,
		}
		if len(fileIDs) > 0 {
			payload["file_ids"] = fileIDs
		}
		headers["Authorization"] = "Bearer " + m.token
		url = m.baseURL() + "/api/v4/posts"
	} else {
		payload = map[string]any{"text": message}

		// An explicit icon wins over the notification type's own image.
		imageURL := m.iconURL
		if imageURL == "" && m.includeImage {
			imageURL = appriseImageURL(notifyType, "72x72")
		}
		if imageURL != "" {
			payload["icon_url"] = imageURL
		}

		username := m.username
		if username == "" {
			username = "Apprise"
		}
		payload["username"] = username

		if channel.value != "" {
			payload["channel"] = channel.value
		}

		url = fmt.Sprintf("%s/hooks/%s", m.baseURL(), m.token)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(43, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"botname": map[string]any{
					"alias_of": "user",
				},
				"channel": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"channels": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"icon_url": map[string]any{
					"map_to":   "icon_url",
					"name":     "Icon URL",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"mode": map[string]any{
					"default":  "webhook",
					"map_to":   "mode",
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"webhook", "bot"},
				},
				"team": map[string]any{
					"alias_of": "user",
				},
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
				"image": map[string]any{
					"default":  true,
					"map_to":   "include_image",
					"name":     "Include Image",
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{host}/{token}", "{schema}://{host}:{port}/{token}", "{schema}://{host}/{fullpath}/{token}", "{schema}://{host}:{port}/{fullpath}/{token}", "{schema}://{user}@{host}/{token}", "{schema}://{user}@{host}:{port}/{token}", "{schema}://{user}@{host}/{fullpath}/{token}", "{schema}://{user}@{host}:{port}/{fullpath}/{token}"},
			"tokens": map[string]any{
				"fullpath": map[string]any{
					"map_to":   "fullpath",
					"name":     "Path",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
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
					"values":   []string{"mmost", "mmosts"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_channel_id": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel ID",
					"prefix":   "",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_channel", "target_channel_id"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"required": true,
					"type":     "string",
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
		"protocols": []string{"mmost"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"mmosts"},
		"service_name":     "Mattermost",
		"service_url":      "https://mattermost.com/",
		"setup_url":        "https://appriseit.com/services/mattermost/",
	})
}
