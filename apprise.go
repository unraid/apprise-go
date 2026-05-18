package apprise

import (
	"errors"
	"fmt"
	"strings"

	"github.com/unraid/apprise-go/internal/notify"
)

// NotifyType identifies the semantic type of a notification.
type NotifyType = notify.NotifyType

const (
	// NotifyInfo sends an informational notification.
	NotifyInfo = notify.NotifyInfo
	// NotifySuccess sends a success notification.
	NotifySuccess = notify.NotifySuccess
	// NotifyWarning sends a warning notification.
	NotifyWarning = notify.NotifyWarning
	// NotifyFailure sends a failure notification.
	NotifyFailure = notify.NotifyFailure
)

// ErrNoTargets is returned when Send is called without any configured URLs.
var ErrNoTargets = errors.New("apprise: no notification URLs configured")

// Option configures a Send call.
type Option func(*notifyOptions)

type notifyOptions struct {
	title       string
	notifyType  NotifyType
	inputFormat string
}

// Apprise stores notification target URLs and sends messages to them.
type Apprise struct {
	urls []string
}

// TargetError wraps a failure from one configured target URL.
type TargetError struct {
	URL string
	Err error
}

func (e *TargetError) Error() string {
	return fmt.Sprintf("%s: %v", e.URL, e.Err)
}

func (e *TargetError) Unwrap() error {
	return e.Err
}

// New creates an empty Apprise client.
func New() *Apprise {
	return &Apprise{}
}

// ParseNotifyType parses an Apprise notification type string.
func ParseNotifyType(raw string) (NotifyType, bool) {
	return notify.ParseNotifyType(raw)
}

// SupportedSchemas returns the URL schemas supported by this build.
func SupportedSchemas() []string {
	return notify.SupportedSchemas()
}

// SupportsSchema reports whether schema is supported by this build.
func SupportsSchema(schema string) bool {
	return notify.SupportsSchema(schema)
}

// WithTitle sets the notification title for a Send call.
func WithTitle(title string) Option {
	return func(opts *notifyOptions) {
		opts.title = title
	}
}

// WithNotifyType sets the semantic notification type for a Send call.
func WithNotifyType(notifyType NotifyType) Option {
	return func(opts *notifyOptions) {
		opts.notifyType = notifyType
	}
}

// WithInputFormat sets the source body format before target-specific conversion.
func WithInputFormat(inputFormat string) Option {
	return func(opts *notifyOptions) {
		opts.inputFormat = inputFormat
	}
}

// Add validates and stores a notification target URL.
func (a *Apprise) Add(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := notify.ParseURL(rawURL)
	if err != nil {
		return err
	}
	if !notify.SupportsSchema(parsed.Scheme) {
		return fmt.Errorf("unsupported url schema: %s", parsed.Scheme)
	}
	a.urls = append(a.urls, rawURL)
	return nil
}

// AddAll validates and stores multiple notification target URLs.
func (a *Apprise) AddAll(rawURLs ...string) error {
	for _, rawURL := range rawURLs {
		if err := a.Add(rawURL); err != nil {
			return err
		}
	}
	return nil
}

// URLs returns a copy of the configured notification target URLs.
func (a *Apprise) URLs() []string {
	urls := make([]string, len(a.urls))
	copy(urls, a.urls)
	return urls
}

// Send sends body to all configured target URLs.
func (a *Apprise) Send(body string, options ...Option) error {
	if len(a.urls) == 0 {
		return ErrNoTargets
	}

	opts := defaultNotifyOptions()
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}

	var errs []error
	for _, rawURL := range a.urls {
		if err := notify.SendTargetURL(rawURL, body, opts.title, opts.inputFormat, opts.notifyType); err != nil {
			errs = append(errs, &TargetError{
				URL: rawURL,
				Err: err,
			})
		}
	}
	return errors.Join(errs...)
}

// Send sends body to rawURLs without explicitly creating an Apprise client.
func Send(rawURLs []string, body string, options ...Option) error {
	client := New()
	if err := client.AddAll(rawURLs...); err != nil {
		return err
	}
	return client.Send(body, options...)
}

func defaultNotifyOptions() notifyOptions {
	return notifyOptions{
		notifyType:  NotifyInfo,
		inputFormat: "text",
	}
}
