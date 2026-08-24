package database

import "testing"

func TestLoadMigrationsOrderedAndComplete(t *testing.T) {
	set, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(set) < 13 {
		t.Fatalf("expected at least the 13 historical migrations, got %d", len(set))
	}
	if set[0].Version != "001" || set[0].Name != "initial" {
		t.Fatalf("first migration should be 001_initial, got %s_%s", set[0].Version, set[0].Name)
	}
	for i := 1; i < len(set); i++ {
		if set[i].Version <= set[i-1].Version {
			t.Fatalf("migrations out of order: %s after %s", set[i].Version, set[i-1].Version)
		}
	}
	for _, m := range set {
		if m.SQL == "" {
			t.Fatalf("%s_%s has empty SQL", m.Version, m.Name)
		}
		if len(m.Sum) != 64 {
			t.Fatalf("%s_%s has malformed checksum %q", m.Version, m.Name, m.Sum)
		}
	}
}
