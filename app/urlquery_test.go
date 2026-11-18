package app

import (
	"reflect"
	"testing"
)

func TestParseURLQuery(t *testing.T) {
	got := ParseURLQuery("https://api.test/a?b=hello+world&a=1&a=2&empty=&flag#frag")
	want := []Header{
		{Name: "b", Value: "hello world"},
		{Name: "a", Value: "1"},
		{Name: "a", Value: "2"},
		{Name: "empty", Value: ""},
		{Name: "flag", Value: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query = %#v, want %#v", got, want)
	}
	if got := ParseURLQuery("localhost:8080/a?q=1"); len(got) != 1 || got[0].Name != "q" || got[0].Value != "1" {
		t.Fatalf("scheme-less query = %#v", got)
	}
}

func TestSetURLQuery(t *testing.T) {
	got := SetURLQuery("https://api.test/a?old=1#frag", []Header{
		{Name: "b", Value: "hello world"},
		{Name: "token", Value: "{{token}}"},
		{Name: "a", Value: "x&y"},
	})
	want := "https://api.test/a?b=hello+world&token={{token}}&a=x%26y#frag"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
	if got := SetURLQuery("localhost:8080/a?q=1", nil); got != "localhost:8080/a" {
		t.Fatalf("remove query = %q", got)
	}
}

func TestResolveURLVariables(t *testing.T) {
	got := ResolveURLVariables("https://api.test/a?q={{q}}&raw=a%26b&flag#{{frag}}", map[string]string{
		"q":    "hello world&x=1",
		"frag": "done now",
	})
	want := "https://api.test/a?q=hello+world%26x%3D1&raw=a%26b&flag=#done now"
	if got != want {
		t.Fatalf("resolved = %q, want %q", got, want)
	}
}
