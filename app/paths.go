package app

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Paths is where api-z keeps its data. The layout follows XDG on Linux,
// %APPDATA% on Windows and Application Support on macOS, with env overrides
// for tests and sandboxes.
type Paths struct {
	DataDir   string
	ConfigDir string
	DBPath    string
	EnvPath   string
	Artifacts string
	LogDir    string
}

func DefaultPaths() (Paths, error) {
	data, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	var baseData, baseConfig string
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("APPDATA"); v != "" {
			baseData = v
		} else {
			baseData = filepath.Join(data, "AppData", "Roaming")
		}
		baseConfig = baseData
	case "darwin":
		baseData = filepath.Join(data, "Library", "Application Support")
		baseConfig = filepath.Join(data, "Library", "Application Support")
	default:
		if v := os.Getenv("XDG_DATA_HOME"); v != "" {
			baseData = v
		} else {
			baseData = filepath.Join(data, ".local", "share")
		}
		if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
			baseConfig = v
		} else {
			baseConfig = filepath.Join(data, ".config")
		}
	}
	return pathsFrom(
		envOr("API_Z_DATA_DIR", filepath.Join(baseData, "api-z")),
		envOr("API_Z_CONFIG_DIR", filepath.Join(baseConfig, "api-z")),
	), nil
}

func pathsFrom(dataDir, configDir string) Paths {
	p := Paths{
		DataDir:   dataDir,
		ConfigDir: configDir,
		DBPath:    filepath.Join(dataDir, "api-z.db"),
		EnvPath:   filepath.Join(dataDir, "environments.json"),
		Artifacts: filepath.Join(dataDir, "request"),
		LogDir:    filepath.Join(dataDir, "logs"),
	}
	return p
}

// PathsAt builds paths explicitly, for -data-dir/-config-dir and tests.
func PathsAt(dataDir, configDir string) Paths {
	return pathsFrom(dataDir, configDir)
}

func (p Paths) Ensure() error {
	for _, dir := range []string{p.DataDir, p.ConfigDir, p.Artifacts, p.LogDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p Paths) RequestDir(id string) string {
	return filepath.Join(p.Artifacts, id)
}

func (p Paths) ResponseDir(id string) string {
	return filepath.Join(p.RequestDir(id), "response")
}

// ApiXDataDir finds the api-x data directory. API_X_DATA_DIR overrides the
// platform default; debugHome in app-settings.properties redirects it, as
// api-x itself does. The directory is only read from.
func ApiXDataDir() string {
	if v := strings.TrimSpace(os.Getenv("API_X_DATA_DIR")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	var dir string
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("APPDATA"); v != "" {
			dir = filepath.Join(v, "api-x")
		} else {
			dir = filepath.Join(home, "AppData", "Roaming", "api-x")
		}
	case "darwin":
		dir = filepath.Join(home, "Library", "Application Support", "api-x")
	default:
		if v := os.Getenv("XDG_DATA_HOME"); v != "" {
			dir = filepath.Join(v, "api-x")
		} else {
			dir = filepath.Join(home, ".local", "share", "api-x")
		}
	}
	if redirected := readDebugHome(dir); redirected != "" {
		return redirected
	}
	return dir
}

func readDebugHome(dir string) string {
	f, err := os.Open(filepath.Join(dir, "app-settings.properties"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == "debugHome" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
