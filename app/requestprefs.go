package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// RequestPrefs controls request timeouts. ReadTimeoutMS is the time allowed
// until the response headers/first byte arrive; TotalTimeoutMS bounds the
// whole exchange, including streaming bodies.
type RequestPrefs struct {
	ReadTimeoutMS  int `json:"readTimeoutMs"`
	TotalTimeoutMS int `json:"totalTimeoutMs"`
}

const (
	DefaultReadTimeoutMS  = 120_000
	DefaultTotalTimeoutMS = 120_000
	MinRequestTimeoutMS   = 1_000
	MaxRequestTimeoutMS   = 86_400_000
)

func DefaultRequestPrefs() RequestPrefs {
	return RequestPrefs{ReadTimeoutMS: DefaultReadTimeoutMS, TotalTimeoutMS: DefaultTotalTimeoutMS}
}

func RequestPrefsPath(configDir string) string {
	return filepath.Join(configDir, "request.json")
}

func NormalizeRequestPrefs(p RequestPrefs) RequestPrefs {
	clamp := func(v, def int) int {
		if v == 0 {
			return def
		}
		if v < MinRequestTimeoutMS {
			return MinRequestTimeoutMS
		}
		if v > MaxRequestTimeoutMS {
			return MaxRequestTimeoutMS
		}
		return v
	}
	p.ReadTimeoutMS = clamp(p.ReadTimeoutMS, DefaultReadTimeoutMS)
	p.TotalTimeoutMS = clamp(p.TotalTimeoutMS, DefaultTotalTimeoutMS)
	return p
}

func LoadRequestPrefs(path string) RequestPrefs {
	var p RequestPrefs
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultRequestPrefs()
	}
	if json.Unmarshal(data, &p) != nil {
		return DefaultRequestPrefs()
	}
	return NormalizeRequestPrefs(p)
}

func SaveRequestPrefs(path string, p RequestPrefs) error {
	if p.ReadTimeoutMS <= 0 || p.TotalTimeoutMS <= 0 {
		return errors.New("超时时间必须大于 0")
	}
	return writeJSON(path, NormalizeRequestPrefs(p))
}
