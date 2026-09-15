package api

import (
	"errors"
	"fmt"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/licensing"
	"net/http/httptest"
	"testing"
)

func TestDeleteInstanceIdempotency(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{{"deleted", nil, 200}, {"already gone", licensing.ErrInstanceNotFound, 200}, {"wrapped missing", fmt.Errorf("delete: %w", licensing.ErrInstanceNotFound), 200}, {"database error", errors.New("database unavailable"), 500}} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeInstanceDeleteResult(w, tc.err)
			if w.Code != tc.status {
				t.Fatalf("got %d want %d", w.Code, tc.status)
			}
		})
	}
}
