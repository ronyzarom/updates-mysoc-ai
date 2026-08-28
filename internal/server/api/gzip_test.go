package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecompressRequest_InflatesGzipBody(t *testing.T) {
	payload := []byte(`{"hello":"world","big":"` + string(bytes.Repeat([]byte("x"), 1000)) + `"}`)

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
	h := decompressRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") != "" {
			t.Errorf("Content-Encoding should be stripped after inflate")
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		got = b
	}))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !bytes.Equal(got, payload) {
		t.Fatalf("inflated body mismatch:\n got %q\nwant %q", got, payload)
	}
}

func TestDecompressRequest_BoundsDecompressionBomb(t *testing.T) {
	// A tiny gzip payload that inflates far past the ceiling must not be read
	// in full: the capped body returns an error once the limit is crossed.
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(bytes.Repeat([]byte("A"), maxDecompressedBytes+1<<20)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat", &buf)
	req.Header.Set("Content-Encoding", "gzip")

	var readErr error
	var readN int
	h := decompressRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		readN = len(b)
		readErr = err
	}))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if readErr == nil {
		t.Fatal("expected an error reading a body past the decompression ceiling")
	}
	if readN > maxDecompressedBytes {
		t.Fatalf("read %d bytes, past the %d ceiling", readN, maxDecompressedBytes)
	}
}

func TestDecompressRequest_PassesThroughPlainBody(t *testing.T) {
	payload := []byte(`{"plain":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat", bytes.NewReader(payload))

	var got []byte
	h := decompressRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = b
	}))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !bytes.Equal(got, payload) {
		t.Fatalf("plain body mismatch: got %q want %q", got, payload)
	}
}
