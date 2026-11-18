package app

import (
	"strings"
	"testing"
)

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

func TestNormalizeLegacyLineFormWithReservedCharacters(t *testing.T) {
	body := "seq: 48961776243138774\naccount: 15774632920\npwd: Abc123456\n" +
		"protocol: true\nchannel: account\nscope: zktr-hccm\ncaptchaType: forceSkip\n" +
		"captcha: SFACwjtzNQdy94iIY2PYjdNp0XkT+6KtU/EPcqwZg92StupNyYEq7FSnzYRLVxy6Eccmdo+MGtY7c8jfQz2buA=="
	got := NormalizeFormBody(body)
	for _, want := range []string{
		"seq=48961776243138774",
		"account=15774632920",
		"pwd=Abc123456",
		"protocol=true",
		"channel=account",
		"scope=zktr-hccm",
		"captchaType=forceSkip",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("form body missing %q: %s", want, got)
		}
	}
	if !strings.Contains(got, "captcha=SFACwjtzNQdy94iIY2PYjdNp0XkT%2B6KtU%2FEPcqwZg92StupNyYEq7FSnzYRLVxy6Eccmdo%2BMGtY7c8jfQz2buA%3D%3D") {
		t.Fatalf("captcha not encoded: %s", got)
	}
}
