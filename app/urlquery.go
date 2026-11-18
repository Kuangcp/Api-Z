package app

import (
	"net/url"
	"strings"
)

// ParseURLQuery returns the URL query in its original order, decoding keys
// and values. Duplicate keys and empty values are preserved.
func ParseURLQuery(rawURL string) []Header {
	_, query, _, _, _ := splitRawURL(rawURL)
	return parseRawQuery(query)
}

func parseRawQuery(query string) []Header {
	if query == "" {
		return nil
	}
	var out []Header
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		rawKey, rawValue, hasValue := strings.Cut(part, "=")
		key, err := url.QueryUnescape(rawKey)
		if err != nil {
			key = rawKey
		}
		value := ""
		if hasValue {
			value, err = url.QueryUnescape(rawValue)
			if err != nil {
				value = rawValue
			}
		}
		out = append(out, Header{Name: key, Value: value})
	}
	return out
}

// ResolveURLVariables applies environment variables to the URL and then
// encodes the query components, so a variable value containing spaces, '&'
// or '=' cannot corrupt the final URL.
func ResolveURLVariables(rawURL string, vars map[string]string) string {
	base, query, fragment, hasQuery, hasFragment := splitRawURL(rawURL)
	base = ApplyVariables(base, vars)
	if !hasQuery {
		if hasFragment {
			base += "#" + ApplyVariables(fragment, vars)
		}
		return base
	}
	params := parseRawQuery(query)
	for i := range params {
		params[i].Name = ApplyVariables(params[i].Name, vars)
		params[i].Value = ApplyVariables(params[i].Value, vars)
	}
	out := SetURLQuery(base, params)
	if hasFragment {
		out += "#" + ApplyVariables(fragment, vars)
	}
	return out
}

// SetURLQuery replaces the URL query while keeping the base and fragment
// exactly as written. Variable placeholders such as {{host}} stay literal so
// environment substitution still works when the request is sent.
func SetURLQuery(rawURL string, params []Header) string {
	base, _, fragment, _, hasFragment := splitRawURL(rawURL)
	pairs := make([]string, 0, len(params))
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		pairs = append(pairs, escapeQueryToken(name)+"="+escapeQueryToken(p.Value))
	}
	out := base
	if len(pairs) > 0 {
		out += "?" + strings.Join(pairs, "&")
	}
	if hasFragment {
		out += "#" + fragment
	}
	return out
}

func splitRawURL(rawURL string) (base, query, fragment string, hasQuery, hasFragment bool) {
	rawURL = strings.TrimSpace(rawURL)
	base = rawURL
	if i := strings.IndexByte(base, '#'); i >= 0 {
		base, fragment = base[:i], base[i+1:]
		hasFragment = true
	}
	if i := strings.IndexByte(base, '?'); i >= 0 {
		base, query = base[:i], base[i+1:]
		hasQuery = true
	}
	return base, query, fragment, hasQuery, hasFragment
}

func escapeQueryToken(s string) string {
	matches := placeholderRE.FindAllStringIndex(s, -1)
	if len(matches) == 0 {
		return url.QueryEscape(s)
	}
	var b strings.Builder
	last := 0
	for _, loc := range matches {
		b.WriteString(url.QueryEscape(s[last:loc[0]]))
		b.WriteString(s[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(url.QueryEscape(s[last:]))
	return b.String()
}
