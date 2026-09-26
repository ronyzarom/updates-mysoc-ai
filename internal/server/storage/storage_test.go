package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSaveNewNeverReplaces(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveNew("swf", "1.0.0", "swf.msi", strings.NewReader("original")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveNew("swf", "1.0.0", "swf.msi", strings.NewReader("replacement")); !errors.Is(err, ErrExists) {
		t.Fatalf("second SaveNew must return ErrExists, got %v", err)
	}
	assertContent(t, s, "swf", "1.0.0", "swf.msi", "original")
	assertNoTempFiles(t, s, "swf", "1.0.0")
}

func TestSaveNewConcurrentSingleWinner(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const n = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := []string{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := strings.Repeat(string(rune('a'+i)), 1<<16)
			if _, err := s.SaveNew("mysoc", "2.0.0", "mysoc.tar.gz", strings.NewReader(body)); err == nil {
				mu.Lock()
				winners = append(winners, body)
				mu.Unlock()
			} else if !errors.Is(err, ErrExists) {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if len(winners) != 1 {
		t.Fatalf("exactly one concurrent SaveNew must win, got %d", len(winners))
	}
	assertContent(t, s, "mysoc", "2.0.0", "mysoc.tar.gz", winners[0])
	assertNoTempFiles(t, s, "mysoc", "2.0.0")
}

func TestRename(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save("siemcore", "3.0.0", ".staging-x", strings.NewReader("bytes")); err != nil {
		t.Fatal(err)
	}
	path, err := s.Rename("siemcore", "3.0.0", ".staging-x", "siemcore.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if path != s.GetPath("siemcore", "3.0.0", "siemcore.tar.gz") || s.Exists("siemcore", "3.0.0", ".staging-x") {
		t.Fatal("rename must move the staged file to its final name")
	}
	assertContent(t, s, "siemcore", "3.0.0", "siemcore.tar.gz", "bytes")
}

func assertContent(t *testing.T, s *LocalStorage, product, version, name, want string) {
	t.Helper()
	r, err := s.Get(product, version, name)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, _ := io.ReadAll(r)
	if string(got) != want {
		t.Fatalf("content changed: got %d bytes starting %q", len(got), string(got[:min(8, len(got))]))
	}
}

func assertNoTempFiles(t *testing.T, s *LocalStorage, product, version string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(s.GetPath(product, version, "x")))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".upload-") {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
}
