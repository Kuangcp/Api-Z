package app

import (
	"strings"
	"testing"

	"api-z/httpclient"
)

func TestParseCurlBasic(t *testing.T) {
	r, err := ParseCurl(`curl 'https://api.test/pets?x=1' -H 'Accept: application/json' -H 'X-A: b' -d 'a=1' -d 'b=2'`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "POST" || r.URL != "https://api.test/pets?x=1" {
		t.Fatalf("request = %#v", r)
	}
	if r.HeadersText != "Accept: application/json\nX-A: b" || r.BodyText != "a=1&b=2" {
		t.Fatalf("headers/body = %q / %q", r.HeadersText, r.BodyText)
	}
}

func TestParseCurlAnsiCAndContinuation(t *testing.T) {
	cmd := "curl -X PUT 'https://api.test/x' \\\n  -H \"Content-Type: application/json\" \\\n  --data-raw $'{\"q\":\"\\u4f60\\u597d\"}'"
	r, err := ParseCurl(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "PUT" || r.BodyText != `{"q":"你好"}` {
		t.Fatalf("request = %#v", r)
	}
	if !strings.Contains(r.HeadersText, "Content-Type: application/json") {
		t.Fatalf("headers = %q", r.HeadersText)
	}
}

func TestParseCurlAuthCookieGet(t *testing.T) {
	r, err := ParseCurl(`curl -u user:pass -b 'a=1; b=2' -A 'ua/1' -e 'https://ref' -G -d 'q=1' -d 'x=2' https://api.test/search`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "GET" || r.BodyText != "" {
		t.Fatalf("request = %#v", r)
	}
	if !strings.Contains(r.URL, "?q=1&x=2") {
		t.Fatalf("url = %q", r.URL)
	}
	for _, want := range []string{"Authorization: Basic dXNlcjpwYXNz", "Cookie: a=1; b=2", "User-Agent: ua/1", "Referer: https://ref"} {
		if !strings.Contains(r.HeadersText, want) {
			t.Fatalf("missing %q in %q", want, r.HeadersText)
		}
	}
}

func TestParseCurlJSONMode(t *testing.T) {
	r, err := ParseCurl(`curl --json '{"a":1}' https://api.test/x`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "POST" || !strings.Contains(r.HeadersText, "Content-Type: application/json") || r.BodyText != `{"a":1}` {
		t.Fatalf("request = %#v", r)
	}
}

func TestParseCurlErrors(t *testing.T) {
	if _, err := ParseCurl(`wget https://x`); err == nil {
		t.Fatal("non-curl accepted")
	}
	if _, err := ParseCurl(`curl -F 'a=@f' https://x`); err == nil {
		t.Fatal("multipart accepted")
	}
	if _, err := ParseCurl(`curl -H 'A: b'`); err == nil {
		t.Fatal("missing URL accepted")
	}
}

func TestCurlRequestName(t *testing.T) {
	r := CurlRequest{URL: "https://api.test/v1/pets?x=1"}
	if got := CurlRequestName(r); got != "api.test/v1/pets" {
		t.Fatalf("name = %q", got)
	}
}

func TestFormatCurlRoundTrip(t *testing.T) {
	spec := httpclient.Spec{
		Method: "POST",
		URL:    "https://example.com/api?q=a+b",
		Headers: []httpclient.Header{
			{Name: "Content-Type", Value: "application/json"},
			{Name: "Authorization", Value: "Bearer tok"},
		},
		Body: []byte(`{"msg":"it's ok"}`),
	}
	cmd := FormatCurl(spec)
	if !strings.Contains(cmd, `it'\''s`) {
		t.Fatalf("single quote not escaped: %s", cmd)
	}
	parsed, err := ParseCurl(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Method != "POST" || parsed.URL != spec.URL || parsed.BodyText != string(spec.Body) {
		t.Fatalf("parsed = %#v", parsed)
	}
	if !strings.Contains(parsed.HeadersText, "Authorization: Bearer tok") {
		t.Fatalf("headers = %q", parsed.HeadersText)
	}

	raw := FormatHTTP(spec)
	if !strings.HasPrefix(raw, "POST /api?q=a+b HTTP/1.1\nHost: example.com\n") {
		t.Fatalf("raw = %q", raw)
	}
	if strings.Count(raw, "Host:") != 1 {
		t.Fatalf("host duplicated: %q", raw)
	}
}
