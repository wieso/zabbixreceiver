package zabbixreceiver

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamingDeflateChecksWholeEncodedBody(t *testing.T) {
	record := []byte(`{"host":{"host":"h"},"name":"load","itemid":1,"clock":1,"value":1,"type":0}`)
	for _, tailSize := range []int{1024, 20000} {
		for _, knownLength := range []bool{true, false} {
			t.Run(fmt.Sprintf("tail=%d/known-length=%t", tailSize, knownLength), func(t *testing.T) {
				cfg := validConfig()
				cfg.Mode = "streaming"
				cfg.Streaming.MaxRequestBodySize = 1024
				encoded := append(compressHistory(t, "deflate", record), bytes.Repeat([]byte("x"), tailSize)...)
				next := newRecordingConsumer(t)
				r := newTestReceiver(t, cfg, &fakeAPI{}, next)
				req := httptest.NewRequest("POST", "/v1/history", bytes.NewReader(encoded))
				if !knownLength {
					req.ContentLength = -1
					req.TransferEncoding = []string{"chunked"}
				}
				req.Header.Set("Content-Type", "application/x-ndjson")
				req.Header.Set("Content-Encoding", "deflate")
				w := httptest.NewRecorder()
				r.handleHistory(w, req)
				require.Equal(t, 413, w.Code, w.Body.String())
				require.Empty(t, next.snapshot())
			})
		}
	}
}

// The encoded stream succeeds, then the HTTP body fails. The decoder may stop
// before observing this failure; metrics must not be acknowledged in that case.
func TestStreamingDeflateChecksLateBodyError(t *testing.T) {
	record := []byte(`{"host":{"host":"h"},"name":"load","itemid":1,"clock":1,"value":1,"type":0}`)
	next := newRecordingConsumer(t)
	r := newTestReceiver(t, validConfig(), &fakeAPI{}, next)
	req := httptest.NewRequest("POST", "/v1/history", io.MultiReader(bytes.NewReader(compressHistory(t, "deflate", record)), bodyFailure{}))
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("Content-Encoding", "deflate")
	w := httptest.NewRecorder()
	r.handleHistory(w, req)
	require.Equal(t, 400, w.Code, w.Body.String())
	require.Empty(t, next.snapshot())
}

type bodyFailure struct{}

func (bodyFailure) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
