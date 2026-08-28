package updatersim

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRelayGzipDecode_InflatesBody(t *testing.T) {
	payload := []byte(`{"instance_id":"child-1","children":[]}`)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat", &buf)
	req.Header.Set("Content-Encoding", "gzip")

	var got []byte
	relayGzipDecode(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") != "" {
			t.Errorf("Content-Encoding should be stripped")
		}
		got, _ = io.ReadAll(r.Body)
	})).ServeHTTP(httptest.NewRecorder(), req)

	if !bytes.Equal(got, payload) {
		t.Fatalf("inflated mismatch: got %q want %q", got, payload)
	}
}
