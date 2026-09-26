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

// 018 was hand-applied to production on 2026-09-18; the runner refuses to
// start if the embedded file's checksum ever differs from that ledger row.
func TestReleaseChannelLengthMatchesProductionLedger(t *testing.T) {
	set, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range set {
		if m.Version == "018" {
			if m.Name != "release_channel_length" || m.Sum != "43ea8681ad9a8717a5000f8384869d112de31cf116a51d2651238d552400b9bc" {
				t.Fatalf("018 must stay byte-identical to production: got %s_%s %s", m.Version, m.Name, m.Sum)
			}
			return
		}
	}
	t.Fatal("migration 018_release_channel_length missing")
}
