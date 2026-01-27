package notify

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

const snsServiceName = "sns"

type SNSTarget struct {
	accessKey string
	secretKey string
	region    string
	phones    []string
	topics    []string
}

func NewSNSTarget(target *ParsedURL) (*SNSTarget, error) {
	accessKey := strings.TrimSpace(target.Host)
	if rawAccess := strings.TrimSpace(target.Query["access"]); rawAccess != "" {
		accessKey = rawAccess
	}
	if accessKey == "" {
		return nil, fmt.Errorf("missing access key")
	}

	entries := splitPath(target.Path)
	secretParts := []string{}
	region := ""
	index := 0
	for i, entry := range entries {
		if awsRegionPattern.MatchString(entry) {
			region = normalizeAWSRegion(entry)
			index = i + 1
			break
		}
		secretParts = append(secretParts, entry)
	}
	if region == "" {
		return nil, fmt.Errorf("missing region")
	}

	secretKey := strings.TrimSpace(strings.Join(secretParts, "/"))
	if rawSecret := strings.TrimSpace(target.Query["secret"]); rawSecret != "" {
		secretKey = rawSecret
	}
	if secretKey == "" {
		return nil, fmt.Errorf("missing secret key")
	}
	if rawRegion := strings.TrimSpace(target.Query["region"]); rawRegion != "" {
		region = normalizeAWSRegion(rawRegion)
	}

	entries = entries[index:]
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	phones := []string{}
	topics := []string{}
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			name := strings.TrimSpace(trimmed[1:])
			if name != "" {
				topics = append(topics, name)
			}
			continue
		}

		if normalized, ok := normalizePhone(trimmed); ok {
			phones = append(phones, "+"+normalized)
			continue
		}

		if isTopicName(trimmed) {
			topics = append(topics, trimmed)
		}
	}

	if len(phones) == 0 && len(topics) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &SNSTarget{
		accessKey: accessKey,
		secretKey: secretKey,
		region:    region,
		phones:    phones,
		topics:    topics,
	}, nil
}

func (s *SNSTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(s.phones) == 0 && len(s.topics) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	payload := ""
	if len(s.phones) > 0 {
		payload = s.publishPhonePayload(message, s.phones[0])
	} else {
		payload = s.createTopicPayload(s.topics[0])
	}

	return RequestSpec{
		Method:  "POST",
		URL:     s.notifyURL(),
		Headers: s.signer().headers(payload, fixedTime()),
		Body:    payload,
	}, nil
}

func (s *SNSTarget) Send(body, title string, notifyType NotifyType) error {
	message := mergeTitleBody(title, body)
	for _, phone := range s.phones {
		payload := s.publishPhonePayload(message, phone)
		spec := RequestSpec{
			Method:  "POST",
			URL:     s.notifyURL(),
			Headers: s.signer().headers(payload, fixedTime()),
			Body:    payload,
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	for _, topic := range s.topics {
		topicArn, err := s.createTopic(topic)
		if err != nil {
			return err
		}
		if topicArn == "" {
			return fmt.Errorf("missing topic arn")
		}
		payload := s.publishTopicPayload(message, topicArn)
		spec := RequestSpec{
			Method:  "POST",
			URL:     s.notifyURL(),
			Headers: s.signer().headers(payload, fixedTime()),
			Body:    payload,
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = title
	_ = notifyType
	return nil
}

func (s *SNSTarget) notifyURL() string {
	return fmt.Sprintf("https://sns.%s.amazonaws.com/", s.region)
}

func (s *SNSTarget) signer() awsSigV4 {
	return awsSigV4{
		accessKey: s.accessKey,
		secretKey: s.secretKey,
		region:    s.region,
		service:   snsServiceName,
		host:      fmt.Sprintf("sns.%s.amazonaws.com", s.region),
	}
}

func (s *SNSTarget) publishPhonePayload(body, phone string) string {
	pairs := []formPair{
		{key: "Action", value: "Publish"},
		{key: "Message", value: body},
		{key: "Version", value: "2010-03-31"},
		{key: "PhoneNumber", value: phone},
	}
	return encodeFormPairs(pairs)
}

func (s *SNSTarget) createTopicPayload(topic string) string {
	pairs := []formPair{
		{key: "Action", value: "CreateTopic"},
		{key: "Version", value: "2010-03-31"},
		{key: "Name", value: topic},
	}
	return encodeFormPairs(pairs)
}

func (s *SNSTarget) publishTopicPayload(body, topicArn string) string {
	pairs := []formPair{
		{key: "Action", value: "Publish"},
		{key: "Version", value: "2010-03-31"},
		{key: "TopicArn", value: topicArn},
		{key: "Message", value: body},
	}
	return encodeFormPairs(pairs)
}

func (s *SNSTarget) createTopic(topic string) (string, error) {
	payload := s.createTopicPayload(topic)
	spec := RequestSpec{
		Method:  "POST",
		URL:     s.notifyURL(),
		Headers: s.signer().headers(payload, fixedTime()),
		Body:    payload,
	}
	req, err := spec.HTTPRequest()
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &HTTPStatusError{StatusCode: resp.StatusCode}
	}
	var response struct {
		TopicArn string `xml:"CreateTopicResult>TopicArn"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}
	return response.TopicArn, nil
}

func isTopicName(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return value != ""
}

func init() {
	RegisterSchemaEntryOrdered(122, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"access": map[string]any{
					"alias_of": "access_key_id",
				},
				"cto": map[string]any{
					"default":  4,
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
				"overflow": map[string]any{
					"default":  "upstream",
					"map_to":   "overflow",
					"name":     "Overflow Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"split", "truncate", "upstream"},
				},
				"region": map[string]any{
					"alias_of": "region",
				},
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"secret": map[string]any{
					"alias_of": "secret_access_key",
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
			"templates": []string{"{schema}://{access_key_id}/{secret_access_key}/{region}/{targets}"},
			"tokens": map[string]any{
				"access_key_id": map[string]any{
					"map_to":   "access_key_id",
					"name":     "Access Key ID",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"region": map[string]any{
					"map_to":   "region_name",
					"name":     "Region",
					"private":  false,
					"regex":    []string{"^[a-z]{2}-[a-z-]+?-[0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "sns",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"sns"},
				},
				"secret_access_key": map[string]any{
					"map_to":   "secret_access_key",
					"name":     "Secret Access Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"target_phone_no": map[string]any{
					"map_to":   "targets",
					"name":     "Target Phone No",
					"private":  false,
					"regex":    []string{"^[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"target_topic": map[string]any{
					"map_to":   "targets",
					"name":     "Target Topic",
					"prefix":   "#",
					"private":  false,
					"regex":    []string{"^[A-Za-z0-9_-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_phone_no", "target_topic"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
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
		"secure_protocols": []string{"sns"},
		"service_name":     "AWS Simple Notification Service (SNS)",
		"service_url":      "https://aws.amazon.com/sns/",
		"setup_url":        "https://appriseit.com/services/sns/",
	})
}
