package main

import (
	"os"
	"testing"
)

// The GPU switch turns mygo's renderer off before the first frame.
func TestApplySoftwareRender(t *testing.T) {
	t.Setenv("MYGO_GPU", "")
	applySoftwareRender(false)
	if got := os.Getenv("MYGO_GPU"); got != "" {
		t.Fatalf("GPU env set without the setting: %q", got)
	}
	applySoftwareRender(true)
	if got := os.Getenv("MYGO_GPU"); got != "0" {
		t.Fatalf("GPU env after software rendering: %q", got)
	}
}
