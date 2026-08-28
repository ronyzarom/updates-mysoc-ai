package api

import (
	"compress/gzip"
	"io"
	"net/http"
)

// maxDecompressedBytes bounds the inflated size of a gzipped request body.
// Without it a small, highly compressible payload could inflate to gigabytes
// of JSON fed to the decoder — a decompression-bomb DoS. The ceiling sits well
// above the largest legitimate body (the cascade's 4 MB rollup cap) so real
// traffic is never clipped, while a bomb is stopped early with 413.
const maxDecompressedBytes = 32 << 20 // 32 MiB

// decompressRequest transparently inflates request bodies sent with
// Content-Encoding: gzip so every downstream handler reads plain JSON. The
// cascade client gzips large rollup heartbeats (~10x smaller on the wire);
// this is the receiving half. Bodies without the header pass through
// untouched, so older non-gzipping clients are unaffected.
//
// The inflated stream is capped with http.MaxBytesReader so a gzip bomb cannot
// force unbounded decompression: reads stop at the ceiling and the handler
// sees a bounded error rather than exhausting memory.
func decompressRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" && r.Body != nil {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid gzip request body")
				return
			}
			inflated := &gzipReadCloser{zr: zr, orig: r.Body}
			r.Body = http.MaxBytesReader(w, inflated, maxDecompressedBytes)
			r.Header.Del("Content-Encoding")
			r.Header.Del("Content-Length")
			r.ContentLength = -1
		}
		next.ServeHTTP(w, r)
	})
}

// gzipReadCloser reads the inflated stream and closes both the gzip reader and
// the original body.
type gzipReadCloser struct {
	zr   *gzip.Reader
	orig io.ReadCloser
}

func (g *gzipReadCloser) Read(p []byte) (int, error) { return g.zr.Read(p) }

func (g *gzipReadCloser) Close() error {
	_ = g.zr.Close()
	return g.orig.Close()
}
