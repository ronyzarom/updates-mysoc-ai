package licensing

import (
	"strings"
	"testing"
)

func TestBuildInstanceFilter_PlaceholderNumbering(t *testing.T) {
	where, args := buildInstanceFilter(InstanceListFilter{
		Tier:     "swf",
		Customer: "cust-1",
		Search:   "host",
		Status:   "offline",
	})
	if len(args) != 4 {
		t.Fatalf("got %d args, want 4: %v", len(args), args)
	}
	// Placeholders must be sequential $1..$4 with no gaps/dupes.
	for i := 1; i <= 4; i++ {
		if !strings.Contains(where, "$"+string(rune('0'+i))) {
			t.Fatalf("missing placeholder $%d in %q", i, where)
		}
	}
	if !strings.HasPrefix(where, " WHERE ") {
		t.Fatalf("where clause should start with WHERE: %q", where)
	}
}

func TestBuildInstanceFilter_Empty(t *testing.T) {
	where, args := buildInstanceFilter(InstanceListFilter{})
	if where != "" {
		t.Fatalf("empty filter should yield no WHERE, got %q", where)
	}
	if len(args) != 0 {
		t.Fatalf("empty filter should yield no args, got %v", args)
	}
}

func TestOrderClause_AllowlistedOnly(t *testing.T) {
	// Known key + asc.
	if got := orderClause(InstanceListFilter{Sort: "hostname", SortDir: "asc"}); got != " ORDER BY hostname ASC" {
		t.Fatalf("hostname asc: got %q", got)
	}
	// Unknown key falls back to created_at DESC (never echoes user input).
	got := orderClause(InstanceListFilter{Sort: "hostname; DROP TABLE instances", SortDir: "asc"})
	if strings.Contains(got, "DROP") {
		t.Fatalf("sort must not echo attacker input: %q", got)
	}
	if got != " ORDER BY created_at ASC" {
		t.Fatalf("unknown sort fallback: got %q", got)
	}
	// Unknown dir falls back to DESC.
	if got := orderClause(InstanceListFilter{Sort: "status", SortDir: "sideways"}); got != " ORDER BY "+derivedStatusExpr+" DESC" {
		t.Fatalf("status desc fallback: got %q", got)
	}
}
