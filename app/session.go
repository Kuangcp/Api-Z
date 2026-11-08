package app

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"api-z/httpclient"
)

const (
	maxDisplayLines = 200_000
	maxSSELineBytes = 8 << 20
	notifyInterval  = 33 * time.Millisecond
)

type SessionSnapshot struct {
	ID          string
	Running     bool
	Done        bool
	Cancelled   bool
	Failed      bool
	IsSSE       bool
	Binary      bool
	Truncated   bool
	Status      int
	StatusText  string
	Headers     []string
	DurationMS  int64
	TTFTMS      int64
	Bytes       int64
	Events      int
	LineCount   int
	Partial     string
	Error       string
	ModelVer    uint64
	AguiVer     uint64
	SpoolPath   string
	ContentType string
}

// Session is one request's stream and derived state. The request goroutine
// writes it; the UI reads it through Snapshots and line accessors.
type Session struct {
	mu     sync.Mutex
	paths  Paths
	store  *Store
	client *httpclient.Client
	notify func()

	id       string
	spec     httpclient.Spec
	cancel   context.CancelFunc
	running  bool
	done     bool
	canceled bool
	errMsg   string
	specURL  string

	status      int
	statusText  string
	headers     []string
	contentType string
	isSSE       bool
	binary      bool

	started   time.Time
	finished  time.Time
	ttft      time.Duration
	bytes     int64
	events    int
	spool     *os.File
	spoolPath string

	lines     []string
	carry     []byte
	truncated bool

	sse   httpclient.SSEParser
	agui  *httpclient.AguiState
	model *httpclient.ModelState

	lastNotify time.Time
	doneCh     chan struct{}
	exchangeID string
	harPath    string
}

func NewSession(id string, paths Paths, store *Store, client *httpclient.Client) *Session {
	if client == nil {
		client = httpclient.NewClient()
	}
	return &Session{
		id:     id,
		paths:  paths,
		store:  store,
		client: client,
		agui:   httpclient.NewAguiState(),
		model:  &httpclient.ModelState{},
	}
}

func (s *Session) SetNotify(fn func()) {
	s.mu.Lock()
	s.notify = fn
	s.mu.Unlock()
}

func (s *Session) ID() string { return s.id }

// Start runs spec. A previous run of this session is cancelled first.
func (s *Session) Start(spec httpclient.Spec) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	now := time.Now()
	s.spec = spec
	s.running = true
	s.done = false
	s.canceled = false
	s.errMsg = ""
	s.status = 0
	s.statusText = ""
	s.headers = nil
	s.contentType = ""
	s.isSSE = false
	s.binary = false
	s.started = now
	s.finished = time.Time{}
	s.ttft = 0
	s.bytes = 0
	s.events = 0
	s.lines = nil
	s.carry = nil
	s.truncated = false
	s.sse = httpclient.SSEParser{}
	s.agui = httpclient.NewAguiState()
	s.model = &httpclient.ModelState{}
	s.doneCh = make(chan struct{})
	s.exchangeID = s.id + "-" + fmt.Sprintf("%d", now.UnixMilli())
	harStem := fmt.Sprintf("%d", now.UnixMilli())
	s.harPath = filepath.Join(s.paths.ResponseDir(s.id), harStem+".har")
	s.lastNotify = time.Time{}
	_ = os.MkdirAll(s.paths.ResponseDir(s.id), 0o755)
	if s.spool != nil {
		_ = s.spool.Close()
	}
	spoolPath := filepath.Join(s.paths.ResponseDir(s.id), harStem+".raw")
	f, err := os.OpenFile(spoolPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		s.errMsg = err.Error()
		s.running = false
		s.done = true
		s.finished = now
		close(s.doneCh)
		fn := s.notify
		s.mu.Unlock()
		if fn != nil {
			fn()
		}
		return
	}
	s.spool = f
	s.spoolPath = spoolPath
	ctx := context.Background()
	if spec.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.RequestTimeout)
		s.cancel = cancel
	} else {
		ctx, s.cancel = context.WithCancel(ctx)
	}
	s.mu.Unlock()

	go func() {
		s.client.Execute(ctx, spec, &sessionHandler{s: s})
	}()
}

func (s *Session) Cancel() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.canceled = true
	s.mu.Unlock()
}

func (s *Session) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Session) Wait() {
	s.mu.Lock()
	ch := s.doneCh
	s.mu.Unlock()
	if ch != nil {
		<-ch
	}
}

func (s *Session) Snapshot() SessionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := SessionSnapshot{
		ID:          s.id,
		Running:     s.running,
		Done:        s.done,
		Cancelled:   s.canceled,
		Failed:      s.errMsg != "",
		IsSSE:       s.isSSE,
		Binary:      s.binary,
		Truncated:   s.truncated,
		Status:      s.status,
		StatusText:  s.statusText,
		DurationMS:  s.durationMS(),
		TTFTMS:      s.ttft.Milliseconds(),
		Bytes:       s.bytes,
		Events:      s.events,
		LineCount:   s.lineCount(),
		Partial:     string(s.carry),
		Error:       s.errMsg,
		ModelVer:    s.model.Version(),
		AguiVer:     s.agui.Version(),
		SpoolPath:   s.spoolPath,
		ContentType: s.contentType,
	}
	if s.headers != nil {
		out.Headers = append([]string(nil), s.headers...)
	}
	return out
}

func (s *Session) Line(i int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i >= 0 && i < len(s.lines) {
		return s.lines[i]
	}
	if i == len(s.lines) && len(s.carry) > 0 {
		return strings.ToValidUTF8(string(s.carry), "\uFFFD")
	}
	return ""
}

func (s *Session) CopyText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for i, line := range s.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	if len(s.carry) > 0 {
		if len(s.lines) > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.ToValidUTF8(string(s.carry), "\uFFFD"))
	}
	return b.String()
}

func (s *Session) AguiSnapshot() httpclient.AguiSnapshot { return s.agui.Snapshot() }
func (s *Session) AguiVersion() uint64                   { return s.agui.Version() }
func (s *Session) ModelText() string                     { return s.model.Text() }
func (s *Session) ModelVersion() uint64                  { return s.model.Version() }

func (s *Session) SpoolPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.spoolPath
}

func (s *Session) durationMS() int64 {
	end := s.finished
	if end.IsZero() {
		end = time.Now()
	}
	if s.started.IsZero() {
		return 0
	}
	return end.Sub(s.started).Milliseconds()
}

func (s *Session) lineCount() int {
	if s.done && len(s.carry) == 0 {
		return len(s.lines)
	}
	if len(s.carry) > 0 {
		return len(s.lines) + 1
	}
	return len(s.lines)
}

func (s *Session) maybeNotify() {
	s.mu.Lock()
	if s.notify == nil || time.Since(s.lastNotify) < notifyInterval {
		s.mu.Unlock()
		return
	}
	s.lastNotify = time.Now()
	fn := s.notify
	s.mu.Unlock()
	fn()
}

func (s *Session) notifyNow() {
	s.mu.Lock()
	fn := s.notify
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

type sessionHandler struct{ s *Session }

func (h *sessionHandler) OnHeaders(status int, header http.Header, contentType string, isSSE bool) {
	s := h.s
	s.mu.Lock()
	s.status = status
	s.statusText = http.StatusText(status)
	s.contentType = contentType
	s.isSSE = isSSE
	s.binary = !isSSE && IsBinaryContentType(contentType)
	for name, values := range header {
		if strings.EqualFold(name, ":status") {
			continue
		}
		for _, v := range values {
			s.headers = append(s.headers, name+": "+v)
		}
	}
	sort.SliceStable(s.headers, func(i, j int) bool {
		return strings.ToLower(s.headers[i]) < strings.ToLower(s.headers[j])
	})
	s.mu.Unlock()
	s.notifyNow()
}

func (h *sessionHandler) OnChunk(p []byte) {
	s := h.s
	s.mu.Lock()
	if s.spool != nil {
		_, _ = s.spool.Write(p)
	}
	s.bytes += int64(len(p))
	if s.binary {
		s.mu.Unlock()
		s.maybeNotify()
		return
	}
	s.carry = append(s.carry, p...)
	for {
		i := bytes.IndexByte(s.carry, '\n')
		if i < 0 {
			break
		}
		line := string(s.carry[:i])
		s.carry = s.carry[i+1:]
		s.handleLineLocked(line, time.Now())
	}
	if len(s.carry) > maxSSELineBytes {
		s.truncated = true
		s.handleLineLocked(string(s.carry), time.Now())
		s.carry = s.carry[:0]
	}
	s.mu.Unlock()
	s.maybeNotify()
}

func (h *sessionHandler) OnDone(err error) {
	s := h.s
	s.mu.Lock()
	if len(s.carry) > 0 {
		s.partialLineLocked(string(s.carry), time.Now())
		s.carry = nil
	}
	if ev, ok := s.sse.Flush(time.Now()); ok {
		s.applyEventLocked(ev)
	}
	if err != nil && !s.canceled && !strings.Contains(err.Error(), "context canceled") {
		s.errMsg = err.Error()
	}
	s.running = false
	s.done = true
	s.finished = time.Now()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.spool != nil {
		_ = s.spool.Close()
		s.spool = nil
	}
	duration := s.durationMS()
	events := s.events
	status := s.status
	bytes := s.bytes
	isSSE := s.isSSE
	spec := s.spec
	ttft := s.ttft.Milliseconds()
	errMsg := s.errMsg
	canceled := s.canceled
	harPath := s.harPath
	exchangeID := s.exchangeID
	s.mu.Unlock()

	// Save the exchange and HAR outside the lock; they touch disk.
	if errMsg == "" && !canceled {
		if err := WriteHAR(harPath, HARInput{
			RequestID:  s.id,
			ExchangeID: exchangeID,
			Spec:       spec,
			Status:     status,
			StatusText: http.StatusText(status),
			Headers:    h.headerSnapshot(),
			BodyPath:   s.SpoolPath(),
			DurationMS: duration,
			Bytes:      bytes,
			IsSSE:      isSSE,
			TTFTMS:     ttft,
			EventCount: events,
		}); err != nil {
			LogWarn("har", "write failed", "request", s.id, "err", err.Error())
		} else if s.store != nil {
			_ = s.store.AddExchange(Exchange{
				ID:         exchangeID,
				RequestID:  s.id,
				SavedAt:    time.Now().UnixMilli(),
				Status:     status,
				DurationMS: duration,
				SizeBytes:  bytes,
				IsSSE:      isSSE,
				TTFTMS:     ttft,
				EventCount: events,
				HARPath:    harPath,
			})
		}
	}
	h.s.notifyNow()
	close(s.doneCh)
}

func (h *sessionHandler) headerSnapshot() []string {
	h.s.mu.Lock()
	defer h.s.mu.Unlock()
	return append([]string(nil), h.s.headers...)
}

func (s *Session) handleLineLocked(line string, at time.Time) {
	if !s.isSSE {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data:") || strings.HasPrefix(trimmed, "event:") {
			s.isSSE = true
		}
	}
	if len(s.lines) < maxDisplayLines {
		s.lines = append(s.lines, line)
	} else {
		s.truncated = true
	}
	if !s.isSSE {
		return
	}
	if ev, ok := s.sse.Line(line, at); ok {
		s.applyEventLocked(ev)
	}
}

func (s *Session) partialLineLocked(line string, at time.Time) {
	if len(s.lines) < maxDisplayLines {
		s.lines = append(s.lines, line)
	} else {
		s.truncated = true
	}
	if s.isSSE {
		if ev, ok := s.sse.Line(line, at); ok {
			s.applyEventLocked(ev)
		}
	}
}

func (s *Session) applyEventLocked(ev httpclient.SSEEvent) {
	s.events++
	if s.ttft == 0 {
		s.ttft = time.Since(s.started)
	}
	s.agui.Apply(ev)
	s.model.Apply(ev)
}
