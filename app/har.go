package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"api-z/httpclient"
)

type HARInput struct {
	RequestID  string
	ExchangeID string
	Spec       httpclient.Spec
	Status     int
	StatusText string
	Headers    []string
	BodyPath   string
	DurationMS int64
	Bytes      int64
	IsSSE      bool
	TTFTMS     int64
	EventCount int
}

type harFile struct {
	Log harLog `json:"log"`
}

type harLog struct {
	Version string     `json:"version"`
	Creator harCreator `json:"creator"`
	APIX    harAPIX    `json:"_apiX"`
	Pages   []any      `json:"pages"`
	Entries []harEntry `json:"entries"`
}

type harCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harAPIX struct {
	SavedAtEpochMS         int64  `json:"savedAtEpochMs"`
	ResponseTimeText       string `json:"responseTimeText"`
	ResponseSizeText       string `json:"responseSizeText"`
	RightTabIndex          int    `json:"rightTabIndex"`
	IsSSEResponse          bool   `json:"isSseResponse"`
	ResponseSSEEventCount  string `json:"responseSseEventCountText,omitempty"`
	RequestHeadersFullText string `json:"requestHeadersFullText,omitempty"`
	RequestBodyText        string `json:"requestBodyText,omitempty"`
	TTFTText               string `json:"ttftText,omitempty"`
	TPOTText               string `json:"tpotText,omitempty"`
}

type harEntry struct {
	StartedDateTime string      `json:"startedDateTime"`
	Time            int64       `json:"time"`
	Request         harRequest  `json:"request"`
	Response        harResponse `json:"response"`
	Cache           any         `json:"cache"`
	Timings         harTimings  `json:"timings"`
}

type harRequest struct {
	Method      string         `json:"method"`
	URL         string         `json:"url"`
	HTTPVersion string         `json:"httpVersion"`
	Headers     []harNameValue `json:"headers"`
	HeadersSize int            `json:"headersSize"`
	BodySize    int            `json:"bodySize"`
	PostData    *harPostData   `json:"postData,omitempty"`
}

type harResponse struct {
	Status      int            `json:"status"`
	StatusText  string         `json:"statusText"`
	HTTPVersion string         `json:"httpVersion"`
	Headers     []harNameValue `json:"headers"`
	HeadersSize int            `json:"headersSize"`
	BodySize    int64          `json:"bodySize"`
	Content     harContent     `json:"content"`
}

type harContent struct {
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type harPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type harNameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harTimings struct {
	Send    int64 `json:"send"`
	Wait    int64 `json:"wait"`
	Receive int64 `json:"receive"`
}

// WriteHAR writes an api-x-compatible HAR. The raw body is read from
// BodyPath; bodies over 64 MiB are kept out of the HAR text to avoid
// turning history into a memory bomb, while the raw file stays on disk.
func WriteHAR(path string, in HARInput) error {
	const maxHARBody = 64 << 20
	body := ""
	if in.BodyPath != "" {
		if info, err := os.Stat(in.BodyPath); err == nil && info.Size() <= maxHARBody {
			if b, err := os.ReadFile(in.BodyPath); err == nil {
				body = strings.ToValidUTF8(string(b), "\uFFFD")
			}
		}
	}
	reqHeaders := make([]harNameValue, 0, len(in.Spec.Headers))
	fullHeaders := make([]string, 0, len(in.Spec.Headers))
	for _, h := range in.Spec.Headers {
		reqHeaders = append(reqHeaders, harNameValue{Name: h.Name, Value: h.Value})
		fullHeaders = append(fullHeaders, h.Name+": "+h.Value)
	}
	respHeaders := make([]harNameValue, 0, len(in.Headers))
	for _, line := range in.Headers {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		respHeaders = append(respHeaders, harNameValue{Name: name, Value: strings.TrimSpace(value)})
	}
	started := time.Now().Add(-time.Duration(in.DurationMS) * time.Millisecond)
	mimeType := ""
	for _, h := range in.Headers {
		if strings.HasPrefix(strings.ToLower(h), "content-type:") {
			mimeType = strings.TrimSpace(h[len("content-type:"):])
			break
		}
	}
	doc := harFile{Log: harLog{
		Version: "1.2",
		Creator: harCreator{Name: "api-z", Version: "0.1.0"},
		APIX: harAPIX{
			SavedAtEpochMS:         started.UnixMilli(),
			ResponseTimeText:       formatDurationMS(in.DurationMS),
			ResponseSizeText:       FormatBytes(in.Bytes),
			RightTabIndex:          0,
			IsSSEResponse:          in.IsSSE,
			ResponseSSEEventCount:  eventText(in.EventCount),
			RequestHeadersFullText: strings.Join(fullHeaders, "\n"),
			RequestBodyText:        string(in.Spec.Body),
			TTFTText:               formatOptionalMS(in.TTFTMS),
		},
		Pages: []any{},
		Entries: []harEntry{{
			StartedDateTime: started.Format(time.RFC3339Nano),
			Time:            in.DurationMS,
			Request: harRequest{
				Method:      in.Spec.Method,
				URL:         in.Spec.URL,
				HTTPVersion: "HTTP/1.1",
				Headers:     reqHeaders,
				HeadersSize: -1,
				BodySize:    len(in.Spec.Body),
				PostData:    postData(in.Spec),
			},
			Response: harResponse{
				Status:      in.Status,
				StatusText:  in.StatusText,
				HTTPVersion: "HTTP/1.1",
				Headers:     respHeaders,
				HeadersSize: -1,
				BodySize:    in.Bytes,
				Content: harContent{
					Size:     in.Bytes,
					MimeType: mimeType,
					Text:     body,
				},
			},
			Cache:   nil,
			Timings: harTimings{Wait: in.DurationMS},
		}},
	}}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func postData(spec httpclient.Spec) *harPostData {
	if len(spec.Body) == 0 {
		return nil
	}
	ct := ""
	for _, h := range spec.Headers {
		if strings.EqualFold(h.Name, "Content-Type") {
			ct = h.Value
			break
		}
	}
	return &harPostData{MimeType: ct, Text: string(spec.Body)}
}

func formatDurationMS(ms int64) string {
	if ms <= 0 {
		return "0ms"
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.2fs", float64(ms)/1000)
}

func formatOptionalMS(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return formatDurationMS(ms)
}

func eventText(n int) string {
	if n <= 0 {
		return ""
	}
	return "Event " + strconv.Itoa(n)
}

// HARResponse is the response part of a saved HAR, for the history popover.
type HARResponse struct {
	Status     int
	StatusText string
	Headers    []string
	Body       string
	IsSSE      bool
	DurationMS int64
	SizeBytes  int64
	TTFTMS     int64
	EventCount int
	HARPath    string
}

func ReadHARResponse(path string) (HARResponse, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return HARResponse{}, err
	}
	var doc harFile
	if err := json.Unmarshal(b, &doc); err != nil {
		return HARResponse{}, err
	}
	out := HARResponse{HARPath: path, IsSSE: doc.Log.APIX.IsSSEResponse}
	if len(doc.Log.Entries) > 0 {
		e := doc.Log.Entries[0]
		out.Status = e.Response.Status
		out.StatusText = e.Response.StatusText
		out.DurationMS = e.Time
		out.SizeBytes = e.Response.Content.Size
		if out.SizeBytes == 0 {
			out.SizeBytes = e.Response.BodySize
		}
		out.Body = e.Response.Content.Text
		for _, h := range e.Response.Headers {
			out.Headers = append(out.Headers, h.Name+": "+h.Value)
		}
	}
	out.EventCount = parseEventText(doc.Log.APIX.ResponseSSEEventCount)
	return out, nil
}
