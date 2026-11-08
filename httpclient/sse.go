package httpclient

import (
	"strconv"
	"strings"
	"time"
)

// SSEEvent is one server-sent event after a blank line flushes it.
type SSEEvent struct {
	Event string
	Data  string
	ID    string
	Retry int
	At    time.Time
}

type SSEParser struct {
	event string
	data  []string
	id    string
	retry int
	seen  bool
}

func (p *SSEParser) Line(line string, at time.Time) (SSEEvent, bool) {
	line = strings.TrimSuffix(line, "\r")
	if line == "" {
		if !p.seen {
			return SSEEvent{}, false
		}
		ev := SSEEvent{
			Event: p.event,
			Data:  strings.Join(p.data, "\n"),
			ID:    p.id,
			Retry: p.retry,
			At:    at,
		}
		p.event = ""
		p.data = p.data[:0]
		p.seen = false
		return ev, true
	}
	if strings.HasPrefix(line, ":") {
		return SSEEvent{}, false
	}
	field, value, ok := strings.Cut(line, ":")
	if ok {
		value = strings.TrimPrefix(value, " ")
	} else {
		field, value = line, ""
	}
	switch field {
	case "event":
		p.event = value
		p.seen = true
	case "data":
		p.data = append(p.data, value)
		p.seen = true
	case "id":
		p.id = value
		p.seen = true
	case "retry":
		if n, err := strconv.Atoi(value); err == nil {
			p.retry = n
			p.seen = true
		}
	}
	return SSEEvent{}, false
}

// Flush emits the event still open at EOF, as the SSE spec does.
func (p *SSEParser) Flush(at time.Time) (SSEEvent, bool) {
	return p.Line("", at)
}
