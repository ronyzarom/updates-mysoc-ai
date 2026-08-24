package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/cyfox-labs/updates-mysoc-ai/migrations"
)

// migrationLockKey is the pg_advisory_lock key serializing migration runs.
// Arbitrary but fixed: "mysocupd" as a 64-bit integer.
const migrationLockKey int64 = 0x6d79736f63757064

var migrationFilePattern = regexp.MustCompile(`^(\d+)_(.+)\.up\.sql$`)

// migration is one embedded, ordered schema change.
type migration struct {
	Version string // zero-padded numeric prefix, e.g. "001"
	Name    string // human part, e.g. "initial"
	SQL     string
	Sum     string // sha256 of the file contents
}

// Migrate brings the schema to the head of the embedded migration set.
// It is safe to run on every startup:
//   - a Postgres advisory lock serializes concurrent starters;
//   - each migration runs in its own transaction and is recorded in
//     schema_migrations, so reruns are no-ops;
//   - a pre-existing database that has no ledger (schema managed manually
//     until now) is BASELINED: every embedded migration is recorded as
//     applied without executing, because the live schema is already at head.
//     Only a genuinely empty database gets the full set executed.
func (db *DB) Migrate(ctx context.Context) error {
	set, err := loadMigrations()
	if err != nil {
		return err
	}

	// One session holds the advisory lock for the whole run; use a dedicated
	// connection so the lock and the work share a session.
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("migrate: advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockKey)
	}()

	// to_regclass is not privilege-filtered (unlike information_schema),
	// so detection works no matter which role runs the migration.
	var ledgerExists bool
	if err := conn.QueryRow(ctx,
		`SELECT to_regclass('public.schema_migrations') IS NOT NULL`,
	).Scan(&ledgerExists); err != nil {
		return fmt.Errorf("migrate: check ledger: %w", err)
	}

	if !ledgerExists {
		// No ledger but a populated schema means this database predates the
		// runner and was migrated by hand; its schema is at head by
		// definition of the release that ships this code. Ledger creation
		// and baseline commit atomically so a crash in between can never
		// leave an empty ledger that would route a rerun into apply mode.
		var schemaPopulated bool
		if err := conn.QueryRow(ctx,
			`SELECT to_regclass('public.instances') IS NOT NULL`,
		).Scan(&schemaPopulated); err != nil {
			return fmt.Errorf("migrate: check schema: %w", err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migrate: begin ledger init: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			CREATE TABLE schema_migrations (
				version    TEXT PRIMARY KEY,
				name       TEXT NOT NULL,
				checksum   TEXT NOT NULL,
				baseline   BOOLEAN NOT NULL DEFAULT FALSE,
				applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrate: create ledger: %w", err)
		}
		if schemaPopulated {
			for _, m := range set {
				if _, err := tx.Exec(ctx, `
					INSERT INTO schema_migrations (version, name, checksum, baseline)
					VALUES ($1, $2, $3, TRUE)`, m.Version, m.Name, m.Sum); err != nil {
					_ = tx.Rollback(ctx)
					return fmt.Errorf("migrate: baseline %s_%s: %w", m.Version, m.Name, err)
				}
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migrate: commit ledger init: %w", err)
		}
		if schemaPopulated {
			log.Printf("migrations: baselined existing schema at %s_%s (%d recorded, none executed)",
				set[len(set)-1].Version, set[len(set)-1].Name, len(set))
			return nil
		}
	}

	applied := map[string]string{}
	rows, err := conn.Query(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("migrate: read ledger: %w", err)
	}
	for rows.Next() {
		var v, sum string
		if err := rows.Scan(&v, &sum); err != nil {
			rows.Close()
			return fmt.Errorf("migrate: scan ledger: %w", err)
		}
		applied[v] = sum
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate: ledger rows: %w", err)
	}

	ran := 0
	for _, m := range set {
		if sum, ok := applied[m.Version]; ok {
			// Baselined rows keep the checksum of the file at baseline time;
			// drift in an already-applied file is a release-engineering error.
			if sum != m.Sum {
				return fmt.Errorf("migrate: %s_%s changed after it was applied (ledger %s, file %s) — never edit an applied migration, add a new one",
					m.Version, m.Name, sum[:12], m.Sum[:12])
			}
			continue
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migrate: begin %s_%s: %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(ctx, m.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrate: apply %s_%s: %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO schema_migrations (version, name, checksum)
			VALUES ($1, $2, $3)`, m.Version, m.Name, m.Sum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrate: record %s_%s: %w", m.Version, m.Name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migrate: commit %s_%s: %w", m.Version, m.Name, err)
		}
		log.Printf("migrations: applied %s_%s", m.Version, m.Name)
		ran++
	}

	if ran == 0 {
		log.Printf("migrations: schema at head (%d in ledger)", len(applied))
	} else {
		log.Printf("migrations: %d applied, schema at head %s_%s", ran, set[len(set)-1].Version, set[len(set)-1].Name)
	}
	return nil
}

// loadMigrations reads and orders the embedded *.up.sql files.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrations.Up, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: read embedded set: %w", err)
	}

	var set []migration
	for _, e := range entries {
		m := migrationFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		body, err := fs.ReadFile(migrations.Up, e.Name())
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", e.Name(), err)
		}
		sum := sha256.Sum256(body)
		set = append(set, migration{
			Version: m[1],
			Name:    strings.TrimSuffix(m[2], ".up.sql"),
			SQL:     string(body),
			Sum:     hex.EncodeToString(sum[:]),
		})
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("migrate: no embedded migrations found")
	}

	sort.Slice(set, func(i, j int) bool { return set[i].Version < set[j].Version })
	for i := 1; i < len(set); i++ {
		if set[i].Version == set[i-1].Version {
			return nil, fmt.Errorf("migrate: duplicate migration version %s", set[i].Version)
		}
	}
	return set, nil
}
