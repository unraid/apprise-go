package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const awsContentType = "application/x-www-form-urlencoded; charset=utf-8"

type awsSigV4 struct {
	accessKey string
	secretKey string
	region    string
	service   string
	host      string
}

func (a awsSigV4) headers(payload string, now time.Time) map[string]string {
	amzDate := now.UTC().Format("20060102T150405Z")
	date := now.UTC().Format("20060102")
	scope := fmt.Sprintf("%s/%s/%s/aws4_request", date, a.region, a.service)

	signedHeaders := []headerPair{
		{key: "content-type", value: awsContentType},
		{key: "host", value: a.host},
		{key: "x-amz-date", value: amzDate},
	}

	var canonicalHeaders strings.Builder
	for _, header := range signedHeaders {
		canonicalHeaders.WriteString(header.key)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(header.value)
		canonicalHeaders.WriteByte('\n')
	}

	canonicalRequest := strings.Join([]string{
		"POST",
		"/",
		"",
		canonicalHeaders.String(),
		joinHeaderKeys(signedHeaders),
		sha256Hex(payload),
	}, "\n")

	toSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex(canonicalRequest),
	}, "\n")

	signature := awsSignature(a.secretKey, date, a.region, a.service, toSign)
	authorization := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		a.accessKey,
		scope,
		joinHeaderKeys(signedHeaders),
		signature,
	)

	return map[string]string{
		"User-Agent":     "Apprise",
		"Content-Type":   awsContentType,
		"Content-Length": strconv.Itoa(len(payload)),
		"X-Amz-Date":     amzDate,
		"Authorization":  authorization,
	}
}

func awsSignature(secret, date, region, service, payload string) string {
	key := []byte("AWS4" + secret)
	dateKey := hmacSHA256(key, date)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, service)
	signingKey := hmacSHA256(serviceKey, "aws4_request")
	return hex.EncodeToString(hmacSHA256(signingKey, payload))
}

func sha256Hex(value string) string {
	// codeql[go/weak-sensitive-data-hashing]
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

type headerPair struct {
	key   string
	value string
}

func joinHeaderKeys(headers []headerPair) string {
	keys := make([]string, 0, len(headers))
	for _, header := range headers {
		keys = append(keys, header.key)
	}
	return strings.Join(keys, ";")
}
