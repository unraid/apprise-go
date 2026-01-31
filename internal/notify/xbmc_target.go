package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	xbmcDefaultPort     = 8080
	xbmcRemoteProtocol  = 2
	kodiRemoteProtocol  = 6
	xbmcImageSize       = "128x128"
	xbmcJSONRPCPath     = "/jsonrpc"
	xbmcNotifyTypeInfo  = "info"
	xbmcNotifyTypeWarn  = "warning"
	xbmcNotifyTypeError = "error"
)

type XBMCTarget struct {
	host         string
	port         int
	secure       bool
	user         string
	password     string
	protocol     int
	includeImage bool
	duration     int
}

func NewXBMCTarget(target *ParsedURL) (*XBMCTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}

	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	schema := strings.ToLower(strings.TrimSpace(target.Scheme))
	protocol := kodiRemoteProtocol
	if strings.HasPrefix(schema, "xbmc") {
		protocol = xbmcRemoteProtocol
	}

	secure := strings.HasSuffix(schema, "s")
	port := target.Port
	if !target.HasPort && protocol == xbmcRemoteProtocol {
		port = xbmcDefaultPort
	}

	includeImage := parseBoolWithDefault(target.Query["image"], true)
	duration := 12
	if raw := strings.TrimSpace(target.Query["duration"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			if parsed < 0 {
				parsed = -parsed
			}
			duration = parsed
		}
	}

	return &XBMCTarget{
		host:         host,
		port:         port,
		secure:       secure,
		user:         strings.TrimSpace(target.User),
		password:     target.Password,
		protocol:     protocol,
		includeImage: includeImage,
		duration:     duration,
	}, nil
}

func (x *XBMCTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "GUI.ShowNotification",
		"params": map[string]any{
			"title":       title,
			"message":     body,
			"displaytime": int(x.duration * 1000),
		},
		"id": 1,
	}

	if x.includeImage {
		imageURL := appriseImageURL(notifyType, xbmcImageSize)
		if imageURL != "" {
			payload["params"].(map[string]any)["image"] = imageURL
			if x.protocol != xbmcRemoteProtocol {
				switch notifyType {
				case NotifyFailure:
					payload["type"] = xbmcNotifyTypeError
				case NotifyWarning:
					payload["type"] = xbmcNotifyTypeWarn
				default:
					payload["type"] = xbmcNotifyTypeInfo
				}
			}
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Content-Type": "application/json",
	}
	if x.user != "" {
		headers["Authorization"] = basicAuthHeader(x.user, x.password)
	}

	return RequestSpec{
		Method:  "POST",
		URL:     x.notifyURL(),
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (x *XBMCTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := x.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (x *XBMCTarget) notifyURL() string {
	scheme := "http"
	if x.secure {
		scheme = "https"
	}

	host := x.host
	if x.port > 0 {
		host = host + ":" + strconv.Itoa(x.port)
	}

	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   xbmcJSONRPCPath,
	}

	return u.String()
}
