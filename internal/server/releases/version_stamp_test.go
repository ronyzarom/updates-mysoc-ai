package releases

import (
	"io"
	"testing"
)

func TestIsSelfUpdateProduct(t *testing.T) {
	cases := map[string]bool{
		"updater-linux-amd64": true,
		"updater-linux-arm64": true,
		"updater-windows":     true,
		"siemcore":            false,
		"mysoc":               false,
		"swf":                 false,
		"":                    false,
	}
	for product, want := range cases {
		if got := isSelfUpdateProduct(product); got != want {
			t.Errorf("isSelfUpdateProduct(%q) = %v, want %v", product, got, want)
		}
	}
}

// writeInChunks feeds b to the scanner in fixed-size Writes so we exercise the
// cross-chunk carry-over of the needle tail.
func writeInChunks(w io.Writer, b []byte, chunk int) {
	for i := 0; i < len(b); i += chunk {
		end := i + chunk
		if end > len(b) {
			end = len(b)
		}
		_, _ = w.Write(b[i:end])
	}
}

func TestVersionStampScanner(t *testing.T) {
	const version = "1.18.0.1"
	// A fake "binary": random-ish bytes with the stamp embedded in the middle,
	// deliberately straddling several small chunk boundaries.
	blob := make([]byte, 0, 4096)
	for i := 0; i < 2000; i++ {
		blob = append(blob, byte(i%251))
	}
	blob = append(blob, []byte("Version="+version+"\x00")...)
	for i := 0; i < 2000; i++ {
		blob = append(blob, byte((i*7)%251))
	}

	for _, chunk := range []int{1, 3, 7, 64, 4096, 100000} {
		s := newVersionStampScanner(version)
		writeInChunks(s, blob, chunk)
		if !s.Found() {
			t.Errorf("chunk=%d: stamp %q not found but present", chunk, version)
		}
	}

	// Wrong version must not be found.
	for _, chunk := range []int{1, 7, 64} {
		s := newVersionStampScanner("1.18.0.2")
		writeInChunks(s, blob, chunk)
		if s.Found() {
			t.Errorf("chunk=%d: wrong version reported as found", chunk)
		}
	}

	// A binary with no stamp at all.
	noStamp := make([]byte, 5000)
	for i := range noStamp {
		noStamp[i] = byte(i % 251)
	}
	s := newVersionStampScanner(version)
	writeInChunks(s, noStamp, 13)
	if s.Found() {
		t.Error("unstamped blob reported as found")
	}
}

func TestVersionStampScannerEmptyNeedle(t *testing.T) {
	// Defensive: an empty declared version never matches (never falsely passes).
	s := newVersionStampScanner("")
	writeInChunks(s, []byte("anything at all"), 4)
	if s.Found() {
		t.Error("empty needle should never report found")
	}
}
