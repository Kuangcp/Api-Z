package app

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type CurlRequest struct {
	Method      string
	URL         string
	HeadersText string
	ParamsText  string
	BodyText    string
}

// ParseCurl parses a sh-style curl command into request fields.
func ParseCurl(command string) (CurlRequest, error) {
	tokens := shellTokens(command)
	if len(tokens) == 0 {
		return CurlRequest{}, errors.New("空命令")
	}
	first := strings.ToLower(tokens[0])
	if first != "curl" && first != "curl.exe" && !strings.HasSuffix(first, "/curl") {
		return CurlRequest{}, errors.New("不是 sh 风格的 curl 命令")
	}
	var (
		method    string
		rawURL    string
		headers   []string
		bodyParts []string
		user      string
		cookie    string
		userAgent string
		referer   string
		get       bool
		jsonMode  bool
	)
	for i := 1; i < len(tokens); i++ {
		t := tokens[i]
		next := func(name string) (string, error) {
			if i+1 >= len(tokens) {
				return "", fmt.Errorf("curl %s 缺少参数", name)
			}
			i++
			return tokens[i], nil
		}
		switch {
		case t == "-X" || t == "--request":
			v, err := next(t)
			if err != nil {
				return CurlRequest{}, err
			}
			method = strings.ToUpper(v)
		case t == "-H" || t == "--header":
			v, err := next(t)
			if err != nil {
				return CurlRequest{}, err
			}
			headers = append(headers, v)
		case t == "-d" || t == "--data" || t == "--data-raw" || t == "--data-binary" || t == "--data-urlencode":
			v, err := next(t)
			if err != nil {
				return CurlRequest{}, err
			}
			bodyParts = append(bodyParts, v)
		case strings.HasPrefix(t, "--data="):
			bodyParts = append(bodyParts, strings.TrimPrefix(t, "--data="))
		case t == "--json":
			v, err := next(t)
			if err != nil {
				return CurlRequest{}, err
			}
			bodyParts = append(bodyParts, v)
			jsonMode = true
		case t == "--url":
			v, err := next(t)
			if err != nil {
				return CurlRequest{}, err
			}
			rawURL = v
		case t == "-u" || t == "--user":
			v, err := next(t)
			if err != nil {
				return CurlRequest{}, err
			}
			user = v
		case t == "-b" || t == "--cookie":
			v, err := next(t)
			if err != nil {
				return CurlRequest{}, err
			}
			cookie = v
		case t == "-A" || t == "--user-agent":
			v, err := next(t)
			if err != nil {
				return CurlRequest{}, err
			}
			userAgent = v
		case t == "-e" || t == "--referer":
			v, err := next(t)
			if err != nil {
				return CurlRequest{}, err
			}
			referer = v
		case t == "-G" || t == "--get":
			get = true
		case t == "-I" || t == "--head":
			method = "HEAD"
		case t == "-F" || t == "--form":
			return CurlRequest{}, errors.New("暂不支持 curl -F/--form（multipart 尚未实现）")
		case t == "--compressed" || t == "-k" || t == "--insecure" || t == "-s" || t == "--silent" || t == "-S" || t == "--show-error":
			// Accepted but no request field mapping yet.
		case t == "-o" || t == "--output":
			if _, err := next(t); err != nil {
				return CurlRequest{}, err
			}
		case strings.HasPrefix(t, "-"):
			// Unknown flags that take no value are ignored; flags with values
			// are not guessed.
			continue
		default:
			if rawURL == "" && looksLikeURL(t) {
				rawURL = t
			}
		}
	}
	if rawURL == "" {
		return CurlRequest{}, errors.New("未解析到 URL")
	}
	if method == "" {
		if len(bodyParts) > 0 {
			method = "POST"
		} else {
			method = "GET"
		}
	}
	if get {
		method = "GET"
		if len(bodyParts) > 0 {
			rawURL = appendQuery(rawURL, strings.Join(bodyParts, "&"))
			bodyParts = nil
		}
	}
	if user != "" && !hasHeader(headers, "Authorization") {
		headers = append(headers, "Authorization: Basic "+base64.StdEncoding.EncodeToString([]byte(user)))
	}
	if cookie != "" && !hasHeader(headers, "Cookie") {
		headers = append(headers, "Cookie: "+cookie)
	}
	if userAgent != "" && !hasHeader(headers, "User-Agent") {
		headers = append(headers, "User-Agent: "+userAgent)
	}
	if referer != "" && !hasHeader(headers, "Referer") {
		headers = append(headers, "Referer: "+referer)
	}
	if jsonMode && !hasHeader(headers, "Content-Type") {
		headers = append(headers, "Content-Type: application/json")
	}
	return CurlRequest{
		Method:      method,
		URL:         rawURL,
		HeadersText: strings.Join(headers, "\n"),
		BodyText:    strings.Join(bodyParts, "&"),
	}, nil
}

// CurlRequestName derives a short display name from a parsed request.
func CurlRequestName(r CurlRequest) string {
	u, err := url.Parse(r.URL)
	if err == nil && u.Host != "" {
		name := u.Host + u.Path
		if len([]rune(name)) > 60 {
			name = string([]rune(name)[:60]) + "…"
		}
		return name
	}
	if r.URL != "" {
		return r.URL
	}
	return r.Method + " cURL"
}

func looksLikeURL(s string) bool {
	low := strings.ToLower(s)
	return strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") ||
		strings.Contains(s, "://") || strings.HasPrefix(s, "localhost") ||
		strings.HasPrefix(s, "127.0.0.1") || strings.Contains(s, ".")
}

func appendQuery(raw, query string) string {
	if query == "" {
		return raw
	}
	if strings.Contains(raw, "#") {
		base, frag, _ := strings.Cut(raw, "#")
		if strings.Contains(base, "?") {
			return base + "&" + query + "#" + frag
		}
		return base + "?" + query + "#" + frag
	}
	if strings.Contains(raw, "?") {
		return raw + "&" + query
	}
	return raw + "?" + query
}

func hasHeader(headers []string, name string) bool {
	for _, h := range headers {
		k, _, ok := strings.Cut(h, ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), name) {
			return true
		}
	}
	return false
}

func shellTokens(command string) []string {
	s := strings.ReplaceAll(command, "\\\r\n", " ")
	s = strings.ReplaceAll(s, "\\\n", " ")
	var out []string
	var current strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if current.Len() > 0 {
			out = append(out, current.String())
			current.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case !inSingle && !inDouble && ch == '$' && i+1 < len(s) && s[i+1] == '\'':
			i += 2
			var raw strings.Builder
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					raw.WriteByte(s[i])
					raw.WriteByte(s[i+1])
					i += 2
					continue
				}
				if s[i] == '\'' {
					i++
					break
				}
				raw.WriteByte(s[i])
				i++
			}
			current.WriteString(decodeAnsiC(raw.String()))
		case ch == '\\' && !inSingle:
			if i+1 < len(s) {
				i++
				current.WriteByte(s[i])
			}
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r') && !inSingle && !inDouble:
			flush()
		default:
			current.WriteByte(ch)
		}
	}
	flush()
	return out
}

func decodeAnsiC(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' || i+1 >= len(raw) {
			b.WriteByte(raw[i])
			continue
		}
		i++
		switch raw[i] {
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'e', 'E':
			b.WriteByte(0x1b)
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '\\', '\'', '"':
			b.WriteByte(raw[i])
		case 'u':
			if i+4 < len(raw) {
				if v, ok := parseHex(raw[i+1 : i+5]); ok {
					b.WriteRune(rune(v))
					i += 4
					continue
				}
			}
			b.WriteByte('\\')
			b.WriteByte(raw[i])
		case 'x':
			end := i + 1
			for end < len(raw) && end < i+3 && isHex(raw[end]) {
				end++
			}
			if end > i+1 {
				if v, ok := parseHex(raw[i+1 : end]); ok {
					b.WriteByte(byte(v))
					i = end - 1
					continue
				}
			}
			b.WriteByte('\\')
			b.WriteByte(raw[i])
		default:
			b.WriteByte('\\')
			b.WriteByte(raw[i])
		}
	}
	return b.String()
}

func parseHex(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	v := 0
	for _, r := range s {
		var d int
		switch {
		case r >= '0' && r <= '9':
			d = int(r - '0')
		case r >= 'a' && r <= 'f':
			d = int(r-'a') + 10
		case r >= 'A' && r <= 'F':
			d = int(r-'A') + 10
		default:
			return 0, false
		}
		v = v*16 + d
	}
	return v, true
}

func isHex(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}
