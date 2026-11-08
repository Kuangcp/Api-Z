package httpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

// Header is one request header. Disabled headers are dropped by the caller.
type Header struct {
	Name  string
	Value string
}

type Spec struct {
	Method         string
	URL            string
	Headers        []Header
	Body           []byte
	RequestTimeout time.Duration
}

// Handler receives a response as it arrives. Its methods are called from the
// request goroutine only, in order.
type Handler interface {
	OnHeaders(status int, header http.Header, contentType string, isSSE bool)
	OnChunk(p []byte)
	OnDone(err error)
}

type Client struct {
	hc *http.Client
}

func NewClient() *Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		// Let Go ask for gzip and decompress it; other encodings are handled
		// below when the user asked for them explicitly.
		DisableCompression: false,
	}
	return &Client{hc: &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}}
}

func (c *Client) CloseIdleConnections() {
	if c != nil && c.hc != nil {
		if tr, ok := c.hc.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
}

// Execute sends spec and streams the response to h. It returns after
// OnDone; OnDone always runs, including on cancellation and timeout.
func (c *Client) Execute(ctx context.Context, spec Spec, h Handler) {
	if h == nil {
		return
	}
	err := c.execute(ctx, spec, h)
	h.OnDone(err)
}

func (c *Client) execute(ctx context.Context, spec Spec, h Handler) error {
	method := strings.ToUpper(strings.TrimSpace(spec.Method))
	if method == "" {
		method = "GET"
	}
	if strings.TrimSpace(spec.URL) == "" {
		return fmt.Errorf("URL 为空")
	}
	var body io.Reader
	if len(spec.Body) > 0 {
		body = bytes.NewReader(spec.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSpace(spec.URL), body)
	if err != nil {
		return err
	}
	host := ""
	for _, hd := range spec.Headers {
		name := strings.TrimSpace(hd.Name)
		if name == "" {
			continue
		}
		switch strings.ToLower(name) {
		case "host":
			host = hd.Value
		case "content-length", "transfer-encoding", "connection", "upgrade", "expect":
			continue
		default:
			req.Header.Add(name, hd.Value)
		}
	}
	if host != "" {
		req.Host = host
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream")
	h.OnHeaders(resp.StatusCode, resp.Header, contentType, isSSE)

	reader, err := decodedReader(resp)
	if err != nil {
		return err
	}
	buf := make([]byte, 32*1024)
	for {
		n, rerr := reader.Read(buf)
		if n > 0 {
			h.OnChunk(buf[:n])
		}
		if rerr != nil {
			if rerr == io.EOF {
				return nil
			}
			return rerr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func decodedReader(resp *http.Response) (io.Reader, error) {
	if resp.Uncompressed {
		return resp.Body, nil
	}
	encoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	switch encoding {
	case "", "identity":
		return resp.Body, nil
	case "gzip", "x-gzip":
		r, err := newGzipReader(resp.Body)
		if err != nil {
			return nil, err
		}
		return r, nil
	case "deflate":
		return newDeflateReader(resp.Body)
	case "br":
		return brotli.NewReader(resp.Body), nil
	default:
		// Unknown encoding: hand the bytes through rather than lose them.
		return resp.Body, nil
	}
}
