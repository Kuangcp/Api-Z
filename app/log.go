package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	logMu   sync.Mutex
	logFile *os.File
	logDir  string
)

// InitLogs opens a log file for this run. Failures are reported on stderr and
// do not stop the app: logging must never take the app down.
func InitLogs(dir string) error {
	logMu.Lock()
	defer logMu.Unlock()
	logDir = dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	now := time.Now()
	month := filepath.Join(dir, now.Format("2006-01"))
	if err := os.MkdirAll(month, 0o755); err != nil {
		return err
	}
	path := filepath.Join(month, now.Format("2006-01-02")+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	logFile = f
	return nil
}

func CloseLogs() {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}

func LogDir() string {
	logMu.Lock()
	defer logMu.Unlock()
	return logDir
}

func LogInfo(tag, msg string, kv ...any)  { writeLog("INFO", tag, msg, kv...) }
func LogWarn(tag, msg string, kv ...any)  { writeLog("WARN", tag, msg, kv...) }
func LogError(tag, msg string, kv ...any) { writeLog("ERROR", tag, msg, kv...) }

func writeLog(level, tag, msg string, kv ...any) {
	var b strings.Builder
	b.WriteString(time.Now().Format("2006-01-02 15:04:05.000"))
	b.WriteString(" ")
	b.WriteString(level)
	b.WriteString(" [")
	b.WriteString(tag)
	b.WriteString("] ")
	b.WriteString(msg)
	for i := 0; i+1 < len(kv); i += 2 {
		b.WriteString(" ")
		b.WriteString(fmt.Sprint(kv[i]))
		b.WriteString("=")
		b.WriteString(fmt.Sprint(kv[i+1]))
	}
	b.WriteString("\n")
	line := b.String()

	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		_, _ = io.WriteString(logFile, line)
	}
	_, _ = io.WriteString(os.Stderr, line)
}

// RedactURL removes query and userinfo from a URL for logs.
func RedactURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if j := strings.IndexByte(rest, '@'); j >= 0 {
			raw = raw[:i+3] + "***@" + rest[j+1:]
		}
	}
	return raw
}
