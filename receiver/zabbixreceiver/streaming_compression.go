package zabbixreceiver

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
)

var errUnsupportedEncoding = errors.New("unsupported content encoding")

// The caller bounds both the encoded input and the decoded output. Snappy uses
// the block format, as in VictoriaMetrics, so check its size before allocating.
func decodeHistoryBody(body io.Reader, encoding string, limit int64) (io.ReadCloser, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity", "none":
		return io.NopCloser(body), nil
	case "gzip":
		return gzip.NewReader(body)
	case "deflate":
		return zlib.NewReader(body)
	case "zstd":
		// Bound decoder working memory independently of the output byte limit.
		decoder, err := zstd.NewReader(body, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64<<20))
		if err != nil {
			return nil, err
		}
		return decoder.IOReadCloser(), nil
	case "snappy":
		encoded, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		size, err := snappy.DecodedLen(encoded)
		if err != nil {
			return nil, err
		}
		if int64(size) > limit {
			return nil, &http.MaxBytesError{Limit: limit}
		}
		decoded, err := snappy.Decode(nil, encoded)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(decoded)), nil
	default:
		return nil, errUnsupportedEncoding
	}
}
