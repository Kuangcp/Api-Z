package app

import "testing"

func TestNormalizeFormBody(t *testing.T) {
	cases := map[string]string{
		"a=1&b=hello world":    "a=1&b=hello+world",
		"b=a%26b&a=x%2By":      "b=a%26b&a=x%2By",
		"flag&empty=":          "flag&empty=",
		"a: 1\nb: hello world": "a=1&b=hello+world",
		"a=%ZZ&b=2":            "a=%ZZ&b=2",
		"a=%E4%BD%A0%E5%A5%BD": "a=%E4%BD%A0%E5%A5%BD",
	}
	for in, want := range cases {
		if got := NormalizeFormBody(in); got != want {
			t.Errorf("NormalizeFormBody(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildSpecFormBody(t *testing.T) {
	headers := "Content-Type: application/x-www-form-urlencoded"
	spec := BuildSpec("POST", "https://example.com/x", headers, "",
		"name=hello world&tag=a%26b", Auth{Type: "noauth"}, nil)
	if got := string(spec.Body); got != "name=hello+world&tag=a%26b" {
		t.Fatalf("form body = %q", got)
	}
	var hs []Header
	for _, h := range spec.Headers {
		hs = append(hs, Header{Name: h.Name, Value: h.Value})
	}
	if HeaderValue(hs, "Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("headers = %#v", spec.Headers)
	}

	json := BuildSpec("POST", "https://example.com/x", "Content-Type: application/json", "",
		`{"msg":"hello world"}`, Auth{Type: "noauth"}, nil)
	if got := string(json.Body); got != `{"msg":"hello world"}` {
		t.Fatalf("json body encoded: %q", got)
	}
}
