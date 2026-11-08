package app

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type Header struct {
	Name  string
	Value string
}

// ParseHeaders keeps the api-x line format: "Name: Value", "! Name: Value"
// disabled, and orphan lines ignored.
func ParseHeaders(text string) []Header {
	var out []Header
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "! ") {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		out = append(out, Header{Name: name, Value: strings.TrimSpace(value)})
	}
	return out
}

// HeadersForSend drops headers the transport must own.
func HeadersForSend(hs []Header) []Header {
	out := make([]Header, 0, len(hs))
	for _, h := range hs {
		switch strings.ToLower(h.Name) {
		case "content-length", "transfer-encoding", "connection", "upgrade", "expect":
			continue
		}
		out = append(out, h)
	}
	return out
}

func ParseParams(text string) []Header {
	return ParseHeaders(text)
}

// MergeURLWithParams adds the Params tab to a URL, preserving an existing
// query and fragment. Spaces encode as '+', as api-x does for form queries.
func MergeURLWithParams(raw string, params []Header) string {
	if len(params) == 0 {
		return strings.TrimSpace(raw)
	}
	var pairs []string
	for _, p := range params {
		pairs = append(pairs, url.QueryEscape(p.Name)+"="+url.QueryEscape(p.Value))
	}
	encoded := strings.Join(pairs, "&")
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "?" + encoded
	}
	before, fragment := trimmed, ""
	if i := strings.IndexByte(trimmed, '#'); i >= 0 {
		before, fragment = trimmed[:i], trimmed[i:]
	}
	path, query := before, ""
	if i := strings.IndexByte(before, '?'); i >= 0 {
		path, query = before[:i], before[i+1:]
	}
	if query == "" {
		return path + "?" + encoded + fragment
	}
	return path + "?" + query + "&" + encoded + fragment
}

func EnsureDefaultScheme(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return u
	}
	low := strings.ToLower(u)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return u
	}
	return "http://" + u
}

var placeholderRE = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

func ApplyVariables(text string, vars map[string]string) string {
	if text == "" || len(vars) == 0 {
		return text
	}
	result := text
	for range 32 {
		next := placeholderRE.ReplaceAllStringFunc(result, func(m string) string {
			name := strings.TrimSpace(placeholderRE.FindStringSubmatch(m)[1])
			if v, ok := vars[name]; ok {
				return v
			}
			return m
		})
		if next == result {
			return result
		}
		result = next
	}
	return result
}

// ResolveAuth turns the api-x auth subset into headers. apikey in=query is
// handled by ResolveAuthQuery.
func ResolveAuth(auth Auth, vars map[string]string) []Header {
	switch strings.ToLower(auth.Type) {
	case "", "inherit", "noauth":
		return nil
	case "basic":
		user := ApplyVariables(auth.Property(auth.Basic, "username"), vars)
		pass := ApplyVariables(auth.Property(auth.Basic, "password"), vars)
		token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		return []Header{{Name: "Authorization", Value: "Basic " + token}}
	case "bearer":
		token := ApplyVariables(auth.Property(auth.Bearer, "token"), vars)
		return []Header{{Name: "Authorization", Value: "Bearer " + token}}
	case "apikey":
		key := ApplyVariables(auth.Property(auth.APIKey, "key"), vars)
		value := ApplyVariables(auth.Property(auth.APIKey, "value"), vars)
		if key == "" {
			return nil
		}
		if strings.EqualFold(auth.Property(auth.APIKey, "in"), "query") {
			return nil
		}
		return []Header{{Name: key, Value: value}}
	}
	return nil
}

func ResolveAuthQuery(auth Auth, vars map[string]string) []Header {
	if !strings.EqualFold(auth.Type, "apikey") || !strings.EqualFold(auth.Property(auth.APIKey, "in"), "query") {
		return nil
	}
	key := ApplyVariables(auth.Property(auth.APIKey, "key"), vars)
	value := ApplyVariables(auth.Property(auth.APIKey, "value"), vars)
	if key == "" {
		return nil
	}
	return []Header{{Name: key, Value: value}}
}

func HeaderValue(hs []Header, name string) string {
	for _, h := range hs {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func IsJSONContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "json") || strings.Contains(ct, "+json")
}

func IsBinaryContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return false
	}
	if strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") || strings.Contains(ct, "xml") ||
		strings.Contains(ct, "javascript") || strings.Contains(ct, "x-www-form-urlencoded") ||
		strings.HasPrefix(ct, "image/svg") {
		return false
	}
	return strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "audio/") ||
		strings.HasPrefix(ct, "video/") || strings.Contains(ct, "octet-stream") ||
		strings.Contains(ct, "pdf") || strings.Contains(ct, "zip")
}

func JoinHeaders(hs []Header) string {
	var b strings.Builder
	for i, h := range hs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(h.Name)
		b.WriteString(": ")
		b.WriteString(h.Value)
	}
	return b.String()
}

func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
