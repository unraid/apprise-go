package notify

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	dotTextURL  = "https://dot.mindreset.tech/api/open/text"
	dotImageURL = "https://dot.mindreset.tech/api/open/image"

	dotModeText  = "text"
	dotModeImage = "image"
)

type DotTarget struct {
	apikey       string
	deviceID     string
	mode         string
	refreshNow   bool
	signature    string
	icon         string
	imageData    string
	link         string
	border       int
	ditherType   string
	ditherKernel string
}

func NewDotTarget(target *ParsedURL) (*DotTarget, error) {
	apikey := strings.TrimSpace(target.User)
	if apikey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	deviceID := strings.TrimSpace(target.Host)
	if deviceID == "" {
		return nil, fmt.Errorf("missing device id")
	}

	mode := dotModeText
	pathTokens := splitPath(target.Path)
	if len(pathTokens) > 0 {
		candidate := strings.ToLower(pathTokens[0])
		if candidate == dotModeText || candidate == dotModeImage {
			mode = candidate
		}
	}

	refreshNow := parseBoolWithDefault(target.Query["refresh"], true)

	signature := strings.TrimSpace(target.Query["signature"])
	icon := strings.TrimSpace(target.Query["icon"])
	imageData := strings.TrimSpace(target.Query["image"])
	link := strings.TrimSpace(target.Query["link"])

	border := 0
	if rawBorder := strings.TrimSpace(target.Query["border"]); rawBorder != "" {
		if value, err := strconv.Atoi(rawBorder); err == nil {
			border = value
		}
	}

	ditherType := strings.TrimSpace(target.Query["dither_type"])
	if ditherType == "" {
		ditherType = "DIFFUSION"
	}

	ditherKernel := strings.TrimSpace(target.Query["dither_kernel"])
	if ditherKernel == "" {
		ditherKernel = "FLOYD_STEINBERG"
	}

	if mode == dotModeText && imageData != "" {
		imageData = ""
	}

	return &DotTarget{
		apikey:       apikey,
		deviceID:     deviceID,
		mode:         mode,
		refreshNow:   refreshNow,
		signature:    signature,
		icon:         icon,
		imageData:    imageData,
		link:         link,
		border:       border,
		ditherType:   ditherType,
		ditherKernel: ditherKernel,
	}, nil
}

func (d *DotTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	spec, err := d.buildRequest(body, title)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (d *DotTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := d.buildRequest(body, title)
	if err != nil {
		return err
	}

	_ = notifyType

	return SendRequest(spec)
}

func (d *DotTarget) buildRequest(body, title string) (RequestSpec, error) {
	payload := map[string]any{
		"refreshNow": d.refreshNow,
		"deviceId":   d.deviceID,
	}

	requestURL := dotTextURL
	if d.mode == dotModeImage {
		if d.imageData == "" {
			return RequestSpec{}, fmt.Errorf("missing image data")
		}

		payload["image"] = d.imageData
		if d.link != "" {
			payload["link"] = d.link
		}
		payload["border"] = d.border
		payload["ditherType"] = d.ditherType
		payload["ditherKernel"] = d.ditherKernel

		requestURL = dotImageURL
	} else {
		if title != "" {
			payload["title"] = title
		}
		if body != "" {
			payload["message"] = body
		}
		if d.signature != "" {
			payload["signature"] = d.signature
		}
		if d.icon != "" {
			payload["icon"] = d.icon
		}
		if d.link != "" {
			payload["link"] = d.link
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"Authorization": "Bearer " + d.apikey,
			"Content-Type":  "application/json",
			"User-Agent":    "Apprise",
		},
		Body: string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(102, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"border": map[string]any{
					"default":  0,
					"map_to":   "border",
					"max":      1,
					"min":      0,
					"name":     "Border",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"cto": map[string]any{
					"default":  4,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"dither_kernel": map[string]any{
					"default":  "FLOYD_STEINBERG",
					"map_to":   "dither_kernel",
					"name":     "Dither Kernel",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"THRESHOLD", "ATKINSON", "BURKES", "FLOYD_STEINBERG", "SIERRA2", "STUCKI", "JARVIS_JUDICE_NINKE", "DIFFUSION_ROW", "DIFFUSION_COLUMN", "DIFFUSION_2D"},
				},
				"dither_type": map[string]any{
					"default":  "DIFFUSION",
					"map_to":   "dither_type",
					"name":     "Dither Type",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"DIFFUSION", "ORDERED", "NONE"},
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
				"icon": map[string]any{
					"map_to":   "icon",
					"name":     "Icon Base64 (Text API)",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"image": map[string]any{
					"map_to":   "image_data",
					"name":     "Image Base64 (Image API)",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"link": map[string]any{
					"map_to":   "link",
					"name":     "Link",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"refresh": map[string]any{
					"default":  true,
					"map_to":   "refresh_now",
					"name":     "Refresh Now",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"signature": map[string]any{
					"map_to":   "signature",
					"name":     "Text Signature",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			"templates": []string{"{schema}://{apikey}@{device_id}/{mode}/"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"device_id": map[string]any{
					"map_to":   "device_id",
					"name":     "Device Serial Number",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"mode": map[string]any{
					"default":  "text",
					"map_to":   "mode",
					"name":     "API Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"text", "image"},
				},
				"schema": map[string]any{
					"default":  "dot",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"dot"},
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"dot"},
		"service_name":     "Dot.",
		"service_url":      "https://dot.mindreset.tech",
		"setup_url":        "https://appriseit.com/services/dot/",
	})
}
