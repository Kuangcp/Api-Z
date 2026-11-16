package ui

import "testing"

func TestURLVariablePattern(t *testing.T) {
	got := urlVariablePattern.FindAllString("https://{{host}}/api/{{ path }}/x", -1)
	if len(got) != 2 || got[0] != "{{host}}" || got[1] != "{{ path }}" {
		t.Fatalf("matches = %#v", got)
	}
	if m := urlVariablePattern.FindAllString("https://{host}/api", -1); len(m) != 0 {
		t.Fatalf("single braces matched: %#v", m)
	}
}

func TestMethodLabelShortensToFour(t *testing.T) {
	cases := map[string]string{"GET": "GET", "POST": "POST", "OPTIONS": "OPTI", "PATCH": "PATC"}
	for in, want := range cases {
		if got := methodLabel(in); got != want {
			t.Fatalf("methodLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
