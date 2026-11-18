package app

import (
	"net/url"
	"strings"
)

// NormalizeFormBody canonicalizes an
// application/x-www-form-urlencoded body: keys and values are decoded once
// and re-encoded, preserving pair order and duplicate keys. A body that is
// already encoded is therefore safe to send again, while raw values get the
// standard '+' for spaces and percent-encoding for reserved characters.
func NormalizeFormBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if looksLikeFormLines(body) {
		var pairs []string
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
			if line == "" || strings.HasPrefix(line, "! ") {
				continue
			}
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			pairs = append(pairs, url.QueryEscape(strings.TrimSpace(key))+"="+url.QueryEscape(strings.TrimSpace(value)))
		}
		return strings.Join(pairs, "&")
	}
	var pairs []string
	for _, part := range strings.Split(body, "&") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rawKey, rawValue, hasValue := strings.Cut(part, "=")
		key, err := url.QueryUnescape(rawKey)
		if err != nil {
			pairs = append(pairs, part)
			continue
		}
		if !hasValue {
			pairs = append(pairs, url.QueryEscape(key))
			continue
		}
		value, err := url.QueryUnescape(rawValue)
		if err != nil {
			pairs = append(pairs, part)
			continue
		}
		pairs = append(pairs, url.QueryEscape(key)+"="+url.QueryEscape(value))
	}
	return strings.Join(pairs, "&")
}

// looksLikeFormLines supports bodies typed as "key: value" lines before the
// form table existed. Encoded key=value pairs never need this conversion.
func looksLikeFormLines(body string) bool {
	if strings.Contains(body, "&") || strings.Contains(body, "=") {
		return false
	}
	found := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "! "))
		if line == "" {
			continue
		}
		if !strings.Contains(line, ":") {
			return false
		}
		found = true
	}
	return found
}

// IsFormContentType reports whether a Content-Type value selects the
// URL-encoded form body encoding.
func IsFormContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "application/x-www-form-urlencoded")
}
