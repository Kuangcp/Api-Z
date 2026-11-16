package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequestPrefsRoundTrip(t *testing.T) {
	path := RequestPrefsPath(t.TempDir())
	p := RequestPrefs{ReadTimeoutMS: 300000, TotalTimeoutMS: 900000}
	if err := SaveRequestPrefs(path, p); err != nil {
		t.Fatal(err)
	}
	got := LoadRequestPrefs(path)
	if got != p {
		t.Fatalf("prefs = %#v", got)
	}
	if n := NormalizeRequestPrefs(RequestPrefs{ReadTimeoutMS: 1, TotalTimeoutMS: 99999999}); n.ReadTimeoutMS != MinRequestTimeoutMS || n.TotalTimeoutMS != MaxRequestTimeoutMS {
		t.Fatalf("clamp = %#v", n)
	}
	if err := os.WriteFile(filepath.Join(path), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadRequestPrefs(path); got != DefaultRequestPrefs() {
		t.Fatalf("corrupt = %#v", got)
	}
}
