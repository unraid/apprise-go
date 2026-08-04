package notify

import (
	"net/url"
	"sort"
	"strings"
)

// formFields is an ordered list of form fields.
//
// url.Values is a map, so a multipart body built by walking one comes out in
// whatever order the keys happen to sort in. Upstream builds its fields from a
// dictionary and requests writes them in insertion order, so the two sides
// disagree on part order for every service that sends more than one field.
//
// Order is not decoration. A receiver parsing the stream sees the fields in
// the order they arrive, and a service that reads a field before the part that
// depends on it — or that stops at the first file — behaves differently. The
// port matched upstream's field *values* while emitting them in its own order,
// and nothing noticed because the comparison matched parts by name.
type formFields struct {
	names  []string
	values []string
}

// Set replaces the value of an existing field in place, keeping its position,
// or appends the field if it is new. This mirrors url.Values.Set closely
// enough that a caller can be moved over without rethinking its logic.
func (f *formFields) Set(name, value string) {
	for i, existing := range f.names {
		if existing == name {
			f.values[i] = value
			return
		}
	}

	f.Add(name, value)
}

// Add appends a field, leaving any field of the same name in place. Services
// that repeat a field name rely on this.
func (f *formFields) Add(name, value string) {
	f.names = append(f.names, name)
	f.values = append(f.values, value)
}

func (f *formFields) Len() int {
	return len(f.names)
}

// Get returns the first value stored under a name, or the empty string.
func (f *formFields) Get(name string) string {
	for i, existing := range f.names {
		if existing == name {
			return f.values[i]
		}
	}

	return ""
}

// Encode renders the fields as an application/x-www-form-urlencoded body, in
// the order they were added. url.Values.Encode sorts by key, which is a second
// place the port diverged from upstream's insertion order. Form bodies are
// compared as parsed key/value pairs rather than as strings, so nothing was
// catching it there either.
func (f *formFields) Encode() string {
	var out strings.Builder
	for i, name := range f.names {
		if i > 0 {
			out.WriteByte('&')
		}
		out.WriteString(url.QueryEscape(name))
		out.WriteByte('=')
		out.WriteString(url.QueryEscape(f.values[i]))
	}

	return out.String()
}

// sortedKeys orders a map's keys so a body built from one is at least stable
// between runs. It is not upstream's order — that comes from the order the URL
// named the keys, which this port does not carry through parsing.
func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

// Clone returns a copy that can be extended without disturbing the original.
// The zero value shares nothing, but appending to a shallow copy of a slice
// header is the kind of aliasing that works until the day it does not.
func (f *formFields) Clone() formFields {
	return formFields{
		names:  append([]string(nil), f.names...),
		values: append([]string(nil), f.values...),
	}
}
