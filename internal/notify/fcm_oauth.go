package notify

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	fcmOAuthScope         = "https://www.googleapis.com/auth/firebase.messaging"
	fcmOAuthTokenURL      = "https://oauth2.googleapis.com/token"
	fcmOAuthMessageURL    = "https://fcm.googleapis.com/v1/projects/%s/messages:send"
	fcmOAuthTokenGrant    = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	fcmOAuthTokenLifetime = time.Hour
)

type fcmServiceAccount struct {
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	TokenURI     string `json:"token_uri"`
}

type fcmTokenResponse struct {
	AccessToken string `json:"access_token"`
}

func (f *FCMTarget) buildOAuthSpec(body, title string, notifyType NotifyType, recipient, accessToken string) (RequestSpec, error) {
	message := map[string]any{
		"notification": map[string]string{
			"title": title,
			"body":  body,
		},
	}

	if strings.HasPrefix(recipient, "#") {
		message["topic"] = strings.TrimPrefix(recipient, "#")
	} else {
		message["token"] = recipient
	}

	if color, ok := f.resolveColor(notifyType); ok && color != "" {
		message["android"] = map[string]any{
			"notification": map[string]string{
				"color": color,
			},
		}
	}

	image := ""
	if f.imageURL != "" {
		image = f.imageURL
	} else if f.includeImage {
		image = appriseImageURL(notifyType, "256x256")
	}
	if f.includeImage && image != "" {
		message["notification"].(map[string]string)["image"] = image
	}

	if len(f.data) > 0 {
		message["data"] = f.data
	}

	payload := map[string]any{
		"message": message,
	}

	if priority := f.oauthPriorityPayload(); len(priority) > 0 {
		mergeMap(payload, priority)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    fmt.Sprintf(fcmOAuthMessageURL, f.project),
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + accessToken,
		},
		Body: string(data),
	}, nil
}

func (f *FCMTarget) accessToken() (string, error) {
	if f.keyfile == "" {
		return "", fmt.Errorf("missing keyfile")
	}
	account, err := loadFCMServiceAccount(f.keyfile)
	if err != nil {
		return "", err
	}
	if f.project != "" && account.ProjectID != "" && f.project != account.ProjectID {
		return "", fmt.Errorf("project mismatch")
	}
	return fetchFCMAccessToken(account)
}

func loadFCMServiceAccount(path string) (*fcmServiceAccount, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var account fcmServiceAccount
	if err := json.Unmarshal(data, &account); err != nil {
		return nil, err
	}
	if account.ClientEmail == "" || account.PrivateKey == "" {
		return nil, errors.New("invalid keyfile")
	}
	if account.TokenURI == "" {
		account.TokenURI = fcmOAuthTokenURL
	}
	return &account, nil
}

func fetchFCMAccessToken(account *fcmServiceAccount) (string, error) {
	assertion, err := account.jwtAssertion()
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", fcmOAuthTokenGrant)
	form.Set("assertion", assertion)

	req, err := http.NewRequest(http.MethodPost, account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Apprise")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("oauth status: %s", resp.Status)
	}

	var tokenResp fcmTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return "", errors.New("missing access token")
	}
	return tokenResp.AccessToken, nil
}

func (account *fcmServiceAccount) jwtAssertion() (string, error) {
	key, err := parseRSAPrivateKey(account.PrivateKey)
	if err != nil {
		return "", err
	}
	now := fixedTime()
	iat := now.Unix()
	exp := now.Add(fcmOAuthTokenLifetime).Unix()

	headerJSON, err := encodeJWTHeader("RS256", account.PrivateKeyID)
	if err != nil {
		return "", err
	}
	claimsJSON, err := encodeJWTPayload(iat, exp, account.ClientEmail, account.TokenURI, fcmOAuthScope)
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	encodedClaims := base64.RawURLEncoding.EncodeToString([]byte(claimsJSON))

	signingInput := encodedHeader + "." + encodedClaims
	sig, err := signRS256(key, signingInput)
	if err != nil {
		return "", err
	}

	return signingInput + "." + sig, nil
}

func encodeJWTHeader(alg, kid string) (string, error) {
	type jwtHeader struct {
		Typ string  `json:"typ"`
		Alg string  `json:"alg"`
		Kid *string `json:"kid"`
	}
	header := jwtHeader{
		Typ: "JWT",
		Alg: alg,
	}
	if kid != "" {
		header.Kid = &kid
	}
	data, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func encodeJWTPayload(iat, exp int64, iss, aud, scope string) (string, error) {
	type jwtPayload struct {
		Iat   int64  `json:"iat"`
		Exp   int64  `json:"exp"`
		Iss   string `json:"iss"`
		Aud   string `json:"aud"`
		Scope string `json:"scope"`
	}
	data, err := json.Marshal(jwtPayload{
		Iat:   iat,
		Exp:   exp,
		Iss:   iss,
		Aud:   aud,
		Scope: scope,
	})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func signRS256(key *rsa.PrivateKey, signingInput string) (string, error) {
	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseRSAPrivateKey(pemKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, errors.New("invalid pem key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("unsupported private key")
	}
	return key, nil
}

func mergeMap(dst, src map[string]any) {
	for key, value := range src {
		srcMap, ok := value.(map[string]any)
		if !ok {
			dst[key] = value
			continue
		}
		if existing, ok := dst[key].(map[string]any); ok {
			mergeMap(existing, srcMap)
			continue
		}
		dst[key] = srcMap
	}
}

func (f *FCMTarget) oauthPriorityPayload() map[string]any {
	if f.priority == "" {
		return nil
	}

	type urgency struct {
		value string
		level string
	}
	lookup := map[string]urgency{
		"min":    {"NORMAL", "very-low"},
		"low":    {"NORMAL", "low"},
		"normal": {"NORMAL", "normal"},
		"high":   {"HIGH", "high"},
		"max":    {"HIGH", "high"},
	}
	entry, ok := lookup[f.priority]
	if !ok {
		return nil
	}

	return map[string]any{
		"message": map[string]any{
			"android": map[string]any{
				"priority": entry.value,
			},
			"apns": map[string]any{
				"headers": map[string]any{
					"apns-priority": func() string {
						if entry.value == "HIGH" {
							return "10"
						}
						return "5"
					}(),
				},
			},
			"webpush": map[string]any{
				"headers": map[string]any{
					"Urgency": entry.level,
				},
			},
		},
	}
}
