package httpclient

import (
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
)

func newGzipReader(r io.Reader) (io.Reader, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	return gz, nil
}

func newDeflateReader(r io.Reader) (io.Reader, error) {
	// "deflate" is zlib-wrapped in the HTTP spec, but some servers send raw
	// DEFLATE. zlib.NewReader reads the wrapper; a raw stream fails on the
	// first bytes, which we do not retry here to keep the stream readable.
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("deflate: %w", err)
	}
	return zr, nil
}
