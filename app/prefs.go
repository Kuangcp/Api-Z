package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ThemePrefs is persisted at <config>/theme.json. Mode: 0 system, 1 light,
// 2 dark. Accent is "#rrggbb" or empty for the theme's own. Font is a family
// name or empty for the system's. Scale is the interface zoom in percent.
type ThemePrefs struct {
	Mode   int    `json:"mode"`
	Accent string `json:"accent,omitempty"`
	Font   string `json:"font,omitempty"`
	Scale  int    `json:"scale,omitempty"`
}

// Theme scale bounds, in percent. Zero means the default.
const (
	DefaultThemeScale = 100
	MinThemeScale     = 75
	MaxThemeScale     = 150
)

// NormalizeThemePrefs fills defaults and clamps values Load and Save accept.
func NormalizeThemePrefs(p ThemePrefs) ThemePrefs {
	if p.Mode < 0 || p.Mode > 2 {
		p.Mode = 0
	}
	if p.Accent != "" && !ValidHex(p.Accent) {
		p.Accent = ""
	}
	p.Font = strings.TrimSpace(p.Font)
	if p.Scale == 0 {
		p.Scale = DefaultThemeScale
	}
	if p.Scale < MinThemeScale {
		p.Scale = MinThemeScale
	}
	if p.Scale > MaxThemeScale {
		p.Scale = MaxThemeScale
	}
	return p
}

func ThemePath(configDir string) string  { return filepath.Join(configDir, "theme.json") }
func KeymapPath(configDir string) string { return filepath.Join(configDir, "keymap.json") }

func LoadThemePrefs(path string) ThemePrefs {
	var p ThemePrefs
	b, err := os.ReadFile(path)
	if err != nil {
		return NormalizeThemePrefs(ThemePrefs{})
	}
	if json.Unmarshal(b, &p) != nil {
		return NormalizeThemePrefs(ThemePrefs{})
	}
	return NormalizeThemePrefs(p)
}

func SaveThemePrefs(path string, p ThemePrefs) error {
	if p.Accent != "" && !ValidHex(p.Accent) {
		return errors.New("强调色必须是 #rrggbb")
	}
	return writeJSON(path, NormalizeThemePrefs(p))
}

// KeymapPrefs maps action ids to chord strings such as "Ctrl+Enter". Only
// overrides are stored; defaults come from the UI action table.
type KeymapPrefs map[string]string

func LoadKeymap(path string) KeymapPrefs {
	var p KeymapPrefs
	b, err := os.ReadFile(path)
	if err != nil {
		return KeymapPrefs{}
	}
	if json.Unmarshal(b, &p) != nil || p == nil {
		return KeymapPrefs{}
	}
	return p
}

func SaveKeymap(path string, p KeymapPrefs) error {
	if p == nil {
		p = KeymapPrefs{}
	}
	return writeJSON(path, p)
}

func ValidHex(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
