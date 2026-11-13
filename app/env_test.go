package app

import "testing"

func TestEnvEnsureTags(t *testing.T) {
	st := EnvState{Environments: []Environment{
		{ID: "1", Name: "HCCM - Test"},
		{ID: "2", Name: "工作流-Local"},
		{ID: "3", Name: "Prod"},
	}}
	got := st.EnsureTags()
	if got.Environments[0].Tag != "HCCM" || got.Environments[1].Tag != "工作流" || got.Environments[2].Tag != "" {
		t.Fatalf("tags = %#v", got.Environments)
	}
}

func TestEnvironmentColorHintAndClean(t *testing.T) {
	cases := []struct{ name, tag, want string }{
		{"工作流 - Production", "", "red"},
		{"HCCM - Test", "", "green"},
		{"文献 - Local", "", "blue"},
		{"本地开发", "", "blue"},
		{"Dev", "开发", "blue"},
		{"Staging", "", "green"},
		{"", "生产", "red"},
		{"Docs", "", ""},
	}
	for _, tc := range cases {
		if got := EnvironmentColorHint(tc.name, tc.tag); got != tc.want {
			t.Fatalf("hint(%q,%q) = %q, want %q", tc.name, tc.tag, got, tc.want)
		}
	}
	st := EnvState{Environments: []Environment{{ID: "e", Name: "Local", Color: "#22c55e"}, {ID: "x", Name: "Bad", Color: "red"}}}
	clean := st.Clean()
	if clean.Environments[0].Color != "#22c55e" || clean.Environments[1].Color != "" {
		t.Fatalf("clean colors = %#v", clean.Environments)
	}
}

func TestIsSSEContentType(t *testing.T) {
	for _, ct := range []string{"text/event-stream", "text/event-stream; charset=utf-8", "TEXT/EVENT-STREAM"} {
		if !IsSSEContentType(ct) {
			t.Fatalf("%q not SSE", ct)
		}
	}
	for _, ct := range []string{"application/json", "text/plain", ""} {
		if IsSSEContentType(ct) {
			t.Fatalf("%q marked SSE", ct)
		}
	}
}
