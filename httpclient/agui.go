package httpclient

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

type AguiKind int

const (
	AguiText AguiKind = iota
	AguiToolCall
	AguiReasoning
	AguiInterrupt
)

type AguiStatus string

const (
	AguiRunning     AguiStatus = "running"
	AguiFinished    AguiStatus = "finished"
	AguiInterrupted AguiStatus = "interrupted"
	AguiError       AguiStatus = "error"
)

type AguiMessage struct {
	Kind       AguiKind
	ID         string
	Role       string
	Text       string
	ToolCallID string
	ToolName   string
	Args       string
	Result     string
	DurationMS int64
	Complete   bool
}

type AguiSnapshot struct {
	RunID    string
	ThreadID string
	Status   AguiStatus
	Error    string
	Messages []AguiMessage
	Version  uint64
}

type aguiMessage struct {
	kind       AguiKind
	id         string
	role       string
	text       strings.Builder
	toolCallID string
	toolName   string
	args       strings.Builder
	result     strings.Builder
	startedAt  time.Time
	durationMS int64
	complete   bool
}

// AguiState folds AG-UI SSE events into messages incrementally: each event
// costs the size of its delta, never the whole run.
type AguiState struct {
	mu       sync.Mutex
	runID    string
	threadID string
	status   AguiStatus
	errMsg   string
	version  uint64

	messages         []*aguiMessage
	pendingText      *aguiMessage
	pendingReasoning *aguiMessage
	pendingToolByID  map[string]*aguiMessage
}

func NewAguiState() *AguiState {
	return &AguiState{
		status:          AguiRunning,
		pendingToolByID: map[string]*aguiMessage{},
	}
}

func (s *AguiState) Version() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

func (s *AguiState) Snapshot() AguiSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := AguiSnapshot{
		RunID:    s.runID,
		ThreadID: s.threadID,
		Status:   s.status,
		Error:    s.errMsg,
		Version:  s.version,
	}
	out.Messages = make([]AguiMessage, 0, len(s.messages))
	for _, m := range s.messages {
		out.Messages = append(out.Messages, AguiMessage{
			Kind:       m.kind,
			ID:         m.id,
			Role:       m.role,
			Text:       m.text.String(),
			ToolCallID: m.toolCallID,
			ToolName:   m.toolName,
			Args:       m.args.String(),
			Result:     m.result.String(),
			DurationMS: m.durationMS,
			Complete:   m.complete,
		})
	}
	return out
}

func (s *AguiState) Apply(ev SSEEvent) {
	if ev.Data == "" || ev.Data == "[DONE]" {
		return
	}
	obj := map[string]any{}
	dec := json.NewDecoder(strings.NewReader(ev.Data))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		return
	}
	typ := str(obj["type"])
	if typ == "" {
		return
	}
	at := ev.At
	if at.IsZero() {
		at = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.version++
	switch typ {
	case "RUN_STARTED":
		s.runID = str(obj["runId"])
		s.threadID = str(obj["threadId"])
		s.status = AguiRunning
	case "RUN_FINISHED":
		s.status = AguiFinished
		if outcome, ok := obj["outcome"].(map[string]any); ok && str(outcome["type"]) == "interrupt" {
			s.status = AguiInterrupted
			s.finishText()
			s.finishReasoning()
			if list, ok := outcome["interrupts"].([]any); ok {
				for _, item := range list {
					ir, _ := item.(map[string]any)
					md, _ := ir["metadata"].(map[string]any)
					ui, _ := md["ui"].(map[string]any)
					s.messages = append(s.messages, &aguiMessage{
						kind:     AguiInterrupt,
						id:       str(ir["id"]),
						text:     builder(str(ir["message"])),
						toolName: str(md["toolName"]),
						toolCallID: func() string {
							_ = ui
							return str(md["toolCallId"])
						}(),
						complete: true,
					})
				}
			}
		}
	case "RUN_ERROR":
		s.status = AguiError
		s.errMsg = str(obj["message"])
	case "TEXT_MESSAGE_START":
		s.finishReasoning()
		s.pendingText = &aguiMessage{
			kind: AguiText,
			id:   str(obj["messageId"]),
			role: defaultStr(str(obj["role"]), "assistant"),
		}
		s.messages = append(s.messages, s.pendingText)
	case "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_CHUNK":
		delta := str(obj["delta"])
		if delta == "" && typ == "TEXT_MESSAGE_CHUNK" {
			delta = str(obj["text"])
		}
		if s.pendingText == nil {
			s.pendingText = &aguiMessage{
				kind: AguiText,
				id:   str(obj["messageId"]),
				role: defaultStr(str(obj["role"]), "assistant"),
			}
			s.messages = append(s.messages, s.pendingText)
		}
		if delta != "" {
			s.pendingText.text.WriteString(delta)
		}
	case "TEXT_MESSAGE_END":
		s.finishText()
	case "REASONING_START", "REASONING_MESSAGE_START":
		s.finishText()
		s.pendingReasoning = &aguiMessage{
			kind: AguiReasoning,
			id:   str(obj["messageId"]),
		}
		s.messages = append(s.messages, s.pendingReasoning)
	case "REASONING_MESSAGE_CONTENT", "REASONING_MESSAGE_CHUNK":
		if s.pendingReasoning == nil {
			s.pendingReasoning = &aguiMessage{kind: AguiReasoning, id: str(obj["messageId"])}
			s.messages = append(s.messages, s.pendingReasoning)
		}
		s.pendingReasoning.text.WriteString(str(obj["delta"]))
	case "REASONING_MESSAGE_END", "REASONING_END":
		s.finishReasoning()
	case "TOOL_CALL_START":
		s.finishText()
		s.finishReasoning()
		id := defaultStr(str(obj["toolCallId"]), "unknown")
		tc := &aguiMessage{
			kind:       AguiToolCall,
			toolCallID: id,
			toolName:   str(obj["toolCallName"]),
			startedAt:  at,
		}
		s.pendingToolByID[id] = tc
		s.messages = append(s.messages, tc)
	case "TOOL_CALL_ARGS":
		id := str(obj["toolCallId"])
		tc := s.pendingToolByID[id]
		if tc == nil {
			return
		}
		tc.args.WriteString(str(obj["delta"]))
	case "TOOL_CALL_END":
		if tc := s.pendingToolByID[str(obj["toolCallId"])]; tc != nil {
			tc.complete = true
		}
	case "TOOL_CALL_RESULT":
		id := str(obj["toolCallId"])
		tc := s.pendingToolByID[id]
		if tc == nil {
			return
		}
		tc.result.WriteString(str(obj["content"]))
		tc.complete = true
		if !tc.startedAt.IsZero() {
			tc.durationMS = at.Sub(tc.startedAt).Milliseconds()
		}
		delete(s.pendingToolByID, id)
	case "TOOL_CALL_CHUNK":
		id := str(obj["toolCallId"])
		tc := s.pendingToolByID[id]
		if tc == nil {
			tc = &aguiMessage{
				kind:       AguiToolCall,
				toolCallID: id,
				toolName:   str(obj["toolCallName"]),
				startedAt:  at,
			}
			s.pendingToolByID[id] = tc
			s.messages = append(s.messages, tc)
		}
		if d := str(obj["delta"]); d != "" {
			tc.args.WriteString(d)
		} else if a := str(obj["args"]); a != "" {
			tc.args.WriteString(a)
		}
		if tc.toolName == "" {
			tc.toolName = str(obj["toolCallName"])
		}
	case "MESSAGES_SNAPSHOT":
		s.finishText()
		s.finishReasoning()
		s.messages = s.messages[:0]
		if list, ok := obj["messages"].([]any); ok {
			for _, item := range list {
				m, _ := item.(map[string]any)
				text := textOf(m["content"])
				s.messages = append(s.messages, &aguiMessage{
					kind:     AguiText,
					id:       str(m["id"]),
					role:     defaultStr(str(m["role"]), "assistant"),
					text:     builder(text),
					complete: true,
				})
			}
		}
	}
}

func (s *AguiState) finishText() {
	if s.pendingText != nil {
		s.pendingText.complete = true
		s.pendingText = nil
	}
}

func (s *AguiState) finishReasoning() {
	if s.pendingReasoning != nil {
		s.pendingReasoning.complete = true
		s.pendingReasoning = nil
	}
}

// ModelState extracts rendered text from an SSE stream (OpenAI Chat
// Completions, Responses, or a generic {"delta": "..."} event).
type ModelState struct {
	mu      sync.Mutex
	text    strings.Builder
	version uint64
	count   int
}

func (m *ModelState) Apply(ev SSEEvent) {
	if ev.Data == "" || ev.Data == "[DONE]" {
		return
	}
	obj := map[string]any{}
	dec := json.NewDecoder(strings.NewReader(ev.Data))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		return
	}
	text := firstText(obj)
	if text == "" {
		return
	}
	m.mu.Lock()
	m.text.WriteString(text)
	m.version++
	m.count++
	m.mu.Unlock()
}

func (m *ModelState) Text() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.text.String()
}

func (m *ModelState) Version() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version
}

func (m *ModelState) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

func firstText(obj map[string]any) string {
	if v := str(obj["delta"]); v != "" {
		return v
	}
	if v := textOf(obj["delta"]); v != "" {
		return v
	}
	if choices, ok := obj["choices"].([]any); ok && len(choices) > 0 {
		if c, ok := choices[0].(map[string]any); ok {
			if d, ok := c["delta"].(map[string]any); ok {
				if v := str(d["content"]); v != "" {
					return v
				}
			}
			if msg, ok := c["message"].(map[string]any); ok {
				if v := textOf(msg["content"]); v != "" {
					return v
				}
			}
			if v := str(c["text"]); v != "" {
				return v
			}
		}
	}
	if output, ok := obj["output"].([]any); ok {
		for _, item := range output {
			if o, ok := item.(map[string]any); ok {
				if v := textOf(o["content"]); v != "" {
					return v
				}
			}
		}
	}
	if v := textOf(obj["content"]); v != "" {
		return v
	}
	if v := str(obj["text"]); v != "" {
		return v
	}
	if msg, ok := obj["message"].(map[string]any); ok {
		return textOf(msg["content"])
	}
	return ""
}

func textOf(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var b strings.Builder
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				if s := str(m["text"]); s != "" {
					b.WriteString(s)
				}
			}
		}
		return b.String()
	case map[string]any:
		if s := str(x["text"]); s != "" {
			return s
		}
	}
	return ""
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func builder(s string) (b strings.Builder) {
	b.WriteString(s)
	return b
}
