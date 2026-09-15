package updatersim

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreMutationRestoresOnlyPointersAndUnknownErrorsRollback(t *testing.T) {
	marker := `MYSOC_APPLY_RESULT_V1:{"phase":"apply","mutation":"none","code":"prerequisite_failed"}`
	for _, test := range []struct {
		name, output string
		exit         int
		typed        bool
	}{
		{"typed", marker, 78, true},
		{"legacy-exit", marker, 1, false},
		{"missing-marker", "no", 78, false},
		{"child-prefix", "child: " + marker, 78, false},
		{"contradictory-output", marker + "\napplication started", 78, false},
		{"duplicate-marker", marker + "\n" + marker, 78, false},
		{"unknown-code", strings.ReplaceAll(marker, "prerequisite_failed", "other"), 78, false},
		{"wrong-phase", strings.ReplaceAll(marker, "apply", "rollback"), 78, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeTestJSON(t, w, map[string]string{"status": "ok"}) }))
			defer server.Close()
			cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
			e := newFSExecutor(t, t.TempDir())
			dir := t.TempDir()
			archive := filepath.Join(dir, "artifact")
			makeTarGz(t, archive, map[string]string{"app": "fixture"})
			for _, v := range []string{"1.0.0", "1.1.0"} {
				if err := e.Apply(context.Background(), Update{Product: "siemcore", ToVersion: v, ArtifactPath: archive}); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(e.previousFile("siemcore"))
			current := resolveCurrent(t, e, "siemcore")
			output := filepath.Join(dir, "output")
			os.WriteFile(output, []byte(test.output), 0600)
			log := filepath.Join(dir, "calls")
			script := filepath.Join(dir, "executor")
			text := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> '" + log + "'\nif [ \"$1\" = apply ]; then cat '" + output + "'; exit " + fmtInt(test.exit) + "; fi\n"
			os.WriteFile(script, []byte(text), 0700)
			e.RestartCommand = []string{script}
			update := Update{Product: "siemcore", FromVersion: "1.1.0", ToVersion: "1.2.0", ArtifactPath: archive}
			err := e.Apply(context.Background(), update)
			if err == nil {
				t.Fatal("expected rejection")
			}
			var pre *PreMutationError
			if errors.As(err, &pre) != test.typed {
				t.Fatalf("wrong classification: %v", err)
			}
			sim, createErr := NewSimulator(cfg, e, discardLogger())
			if createErr != nil {
				t.Fatal(createErr)
			}
			sim.failAndRollback(context.Background(), update, err)
			calls, _ := os.ReadFile(log)
			if test.typed {
				if string(calls) != "apply\n" {
					t.Fatalf("preflight triggered product rollback: %s", calls)
				}
				after, _ := os.ReadFile(e.previousFile("siemcore"))
				if string(after) != string(before) {
					t.Fatal("lost earlier predecessor")
				}
			} else if string(calls) != "apply\nrollback\n" {
				t.Fatalf("unknown error skipped rollback: %s", calls)
			}
			if resolveCurrent(t, e, "siemcore") != current {
				t.Fatal("current pointer not restored")
			}
		})
	}
}

func fmtInt(n int) string {
	if n == 78 {
		return "78"
	}
	return "1"
}

func TestLocalStagingFailureDoesNotInvokeApplicationRollback(t *testing.T) {
	e := newFSExecutor(t, t.TempDir())
	err := e.Apply(context.Background(), Update{Product: "siemcore", ToVersion: "1.1.0", ArtifactPath: "/missing-test-artifact"})
	var pre *PreMutationError
	if !errors.As(err, &pre) || pre.RestoreError != nil {
		t.Fatalf("expected local pre-mutation failure: %v", err)
	}
	if _, err = os.Lstat(e.currentLink("siemcore")); !os.IsNotExist(err) {
		t.Fatal("fresh failure created current")
	}
	if _, err = os.Lstat(e.previousFile("siemcore")); !os.IsNotExist(err) {
		t.Fatal("fresh failure created previous")
	}
}

func TestFailedRestagingCannotDeleteActiveRelease(t *testing.T) {
	e := newFSExecutor(t, t.TempDir())
	p := filepath.Join(t.TempDir(), "artifact")
	makeTarGz(t, p, map[string]string{"sentinel": "retained"})
	u := Update{Product: "siemcore", ToVersion: "1.0.0", ArtifactPath: p}
	if err := e.Apply(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	u.ArtifactPath = "/missing-test-artifact"
	err := e.Apply(context.Background(), u)
	var pre *PreMutationError
	if !errors.As(err, &pre) {
		t.Fatalf("expected pre-mutation guard: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(e.versionDir("siemcore", "1.0.0"), "sentinel"))
	if err != nil || string(data) != "retained" {
		t.Fatal("active release was cleared")
	}
}
