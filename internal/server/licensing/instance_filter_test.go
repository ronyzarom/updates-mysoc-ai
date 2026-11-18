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
	if where != " WHERE deleted_at IS NULL" {
		t.Fatalf("empty filter should exclude deleted instances, got %q", where)
	}
	if len(args) != 0 {
		t.Fatalf("empty filter should yield no args, got %v", args)
	}
}

func TestOrderClause_AllowlistedOnly(t *testing.T) {
	// Known key + asc, with the stable id tie-breaker.
	if got := orderClause(InstanceListFilter{Sort: "hostname", SortDir: "asc"}); got != " ORDER BY hostname ASC, id ASC" {
		t.Fatalf("hostname asc: got %q", got)
	}
	// Unknown key falls back to created_at DESC (never echoes user input).
	got := orderClause(InstanceListFilter{Sort: "hostname; DROP TABLE instances", SortDir: "asc"})
	if strings.Contains(got, "DROP") {
		t.Fatalf("sort must not echo attacker input: %q", got)
	}
	if got != " ORDER BY created_at ASC, id ASC" {
		t.Fatalf("unknown sort fallback: got %q", got)
	}
	// Unknown dir falls back to DESC.
	if got := orderClause(InstanceListFilter{Sort: "status", SortDir: "sideways"}); got != " ORDER BY "+derivedStatusExpr+" DESC, id DESC" {
		t.Fatalf("status desc fallback: got %q", got)
	}
}

func TestBuildInstanceFilter_UnassignedCustomer(t *testing.T) {
	where, args := buildInstanceFilter(InstanceListFilter{Customer: unassignedCustomer})
	if !strings.Contains(where, "customer_id IS NULL OR customer_id = ''") {
		t.Fatalf("__none__ should match unassigned, got %q", where)
	}
	if len(args) != 0 {
		t.Fatalf("__none__ sentinel takes no args, got %v", args)
	}
}

func TestBuildInstanceFilter_SearchAndFailed(t *testing.T) {
	where, args := buildInstanceFilter(InstanceListFilter{Search: "10%_host", Failed: true})
	// Search spans display_name and last_ip_address, not just id/hostname/customer.
	for _, col := range []string{"instance_id ILIKE", "hostname ILIKE", "display_name ILIKE", "customer_name ILIKE", "last_ip_address ILIKE"} {
		if !strings.Contains(where, col) {
			t.Fatalf("search missing %s in %q", col, where)
		}
	}
	// Wildcards in the term are escaped so they match literally.
	if args[0] != `%10\%\_host%` {
		t.Fatalf("search term not escaped: %q", args[0])
	}
	if !strings.Contains(where, "last_update_success IS FALSE") {
		t.Fatalf("failed filter not applied: %q", where)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`a%b_c\d`); got != `a\%b\_c\\d` {
		t.Fatalf("escapeLike: got %q", got)
	}
}
