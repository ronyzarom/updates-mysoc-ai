package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	p "github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var driver, repo, python string

func TestMain(m *testing.M) {
	cwd, _ := os.Getwd()
	repo = filepath.Clean(filepath.Join(cwd, "../.."))
	python, _ = exec.LookPath("python3")
	root, e := os.MkdirTemp("", "pod-joint-driver-")
	if e != nil {
		panic(e)
	}
	driver = filepath.Join(root, "coordinator")
	cmd := exec.Command("go", "build", "-o", driver, ".")
	if out, e := cmd.CombinedOutput(); e != nil {
		fmt.Fprintln(os.Stderr, string(out), e)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
func put(t *testing.T, path string, v any) {
	t.Helper()
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
}

type fixture struct {
	root, config, proxy string
	c                   Config
	priv, observer      ed25519.PrivateKey
	proxyConfig         map[string]any
}

func setup(t *testing.T, mode, outcome string) fixture {
	t.Helper()
	root := t.TempDir()
	if base := os.Getenv("POD_QUALIFICATION_EVIDENCE_ROOT"); base != "" {
		var err error
		root, err = os.MkdirTemp(base, "case-")
		if err != nil {
			t.Fatal(err)
		}
		t.Log("retained component evidence:", root)
	}
	dir := filepath.Join(root, "journal")
	os.MkdirAll(dir, 0700)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	opub, observer, _ := ed25519.GenerateKey(rand.Reader)
	makeArtifact := func(name, version string) p.RetainedArtifact {
		path := filepath.Join(root, name)
		data := []byte("synthetic signed " + version)
		os.WriteFile(path, data, 0600)
		sha := sha256.Sum256(data)
		digest := hex.EncodeToString(sha[:])
		return p.RetainedArtifact{Product: "siemcore", Version: version, SHA256: digest, Path: path, Signature: signing.Sign(priv, "siemcore", version, digest)}
	}
	target := makeArtifact("target", "2")
	previous := makeArtifact("predecessor", "1")
	b := p.Binding{Protocol: p.Protocol, OperationID: "qualification-operation", PodID: "qualification-pod", NodeID: "1", UpdaterID: "qualification-updater", Product: "siemcore", FromVersion: "1", TargetVersion: "2", ArtifactSHA256: target.SHA256, ArtifactSignature: target.Signature, ArtifactPath: target.Path, PreviousArtifactSHA256: previous.SHA256, Deadline: time.Now().UTC().Add(time.Hour)}
	proxy := filepath.Join(root, "proxy.json")
	pc := map[string]any{"isolated": true, "evidence_directory": filepath.Join(root, "calls"), "adapter_command": []string{python, filepath.Join(repo, "scripts/qualification/pod-maintenance/reference_adapter.py"), filepath.Join(root, "reference")}}
	c := Config{EvidenceTier: "component", Isolated: true, TestID: filepath.Base(root), Mode: mode, Directory: dir, Adapter: []string{python, filepath.Join(repo, "scripts/qualification/pod-maintenance/fault_proxy.py"), "--config", proxy}, TimeoutSeconds: 10, Binding: b, ObserverKey: hex.EncodeToString(opub), ReleaseKey: hex.EncodeToString(pub)}
	if mode == "recovery-v2" {
		b.Deadline = time.Now().UTC().Add(-time.Hour)
		c.Binding = b
		put(t, filepath.Join(dir, "operation.json"), p.Journal{Binding: b, Generation: 7, Phase: "applying"})
		action := "resume-target"
		c.Artifact = target
		if outcome == p.PredecessorRestored {
			action = "restore-predecessor"
			c.Artifact = previous
		}
		claims := p.RecoveryClaims{Protocol: p.RecoveryProtocol, AuthorizationID: "qualification-authorization", Binding: b, Generation: 7, Action: action, IssuedAt: time.Now().UTC().Add(-time.Second), ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}
		raw, _ := json.Marshal(claims)
		c.Authorization = p.RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(observer, append([]byte(p.RecoveryAuthorizationDomain), raw...)))}
	}
	cfg := filepath.Join(root, "config.json")
	put(t, proxy, pc)
	put(t, cfg, c)
	return fixture{root: root, config: cfg, proxy: proxy, c: c, priv: priv, observer: observer, proxyConfig: pc}
}
func (f *fixture) fault(t *testing.T, action, point, effect string) {
	f.proxyConfig["fault"] = map[string]any{"action": action, "point": point, "effect": effect}
	put(t, f.proxy, f.proxyConfig)
}
func (f fixture) run(t *testing.T, success bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, driver, "--config", f.config).CombinedOutput()
	if (e == nil) != success {
		t.Fatalf("success=%v err=%v output=%s", success, e, out)
	}
}
func (f fixture) calls(t *testing.T) []string {
	raw, e := os.ReadFile(filepath.Join(f.root, "reference/reference-state.json"))
	if e != nil {
		t.Fatal(e)
	}
	var s struct {
		Calls []string `json:"calls"`
	}
	json.Unmarshal(raw, &s)
	return s.Calls
}
func TestRealCoordinatorBoundaryMatrix(t *testing.T) {
	for _, mode := range []string{"maintenance-v1", "recovery-v2"} {
		outcomes := []string{p.TargetInstalled}
		actions := []string{"capabilities", "begin-or-resume", "status", "apply", "health", "complete", "acceptance"}
		if mode == "recovery-v2" {
			outcomes = append(outcomes, p.PredecessorRestored)
			actions = []string{"capabilities", "status", "authorize-recovery", "recover", "health", "complete", "acceptance"}
		}
		for _, outcome := range outcomes {
			for _, action := range actions {
				for _, fault := range []struct{ point, effect string }{{"before", "crash"}, {"after", "crash"}, {"after", "lost-response"}} {
					name := mode + "/" + outcome + "/" + action + "/" + fault.point + "-" + fault.effect
					t.Run(name, func(t *testing.T) {
						t.Parallel()
						f := setup(t, mode, outcome)
						f.fault(t, action, fault.point, fault.effect)
						f.run(t, false)
						before, e := p.ReadJournal(f.c.Directory)
						if e != nil {
							t.Fatal(e)
						}
						f.run(t, true)
						after, e := p.ReadJournal(f.c.Directory)
						if e != nil {
							t.Fatal(e)
						}
						if before.Binding != after.Binding {
							t.Fatal("operation identity changed")
						}
						want := "accepted"
						if outcome == p.PredecessorRestored {
							want = "rolled-back"
						}
						if after.Phase != want {
							t.Fatal(after.Phase)
						}
						applyCount := 0
						for _, a := range f.calls(t) {
							if a == "apply" {
								applyCount++
							}
						}
						if applyCount > 1 {
							t.Fatal("apply replayed", f.calls(t))
						}
						if action == "complete" && fault.point == "after" {
							count := 0
							for _, a := range f.calls(t) {
								if a == "complete" {
									count++
								}
							}
							if count != 1 {
								t.Fatal("completion replay", f.calls(t))
							}
						}
					})
				}
			}
		}
	}
}
func TestRealCoordinatorRecoverAfterApplyCrash(t *testing.T) {
	f := setup(t, "maintenance-v1", p.TargetInstalled)
	f.fault(t, "apply", "after", "crash")
	f.run(t, false)
	f.fault(t, "recover", "after", "lost-response")
	f.run(t, false)
	f.run(t, true)
	count := 0
	for _, a := range f.calls(t) {
		if a == "apply" {
			count++
		}
	}
	if count != 1 {
		t.Fatal(f.calls(t))
	}
}
func TestRealCoordinatorRollbackThenArchivalLostReply(t *testing.T) {
	f := setup(t, "recovery-v2", p.PredecessorRestored)
	f.run(t, true)
	old, _ := os.ReadFile(filepath.Join(f.c.Directory, "operation.json"))
	j, e := p.ReadJournal(f.c.Directory)
	if e != nil {
		t.Fatal(e)
	}
	next := j.Binding
	next.OperationID = "qualification-next-operation"
	next.TargetVersion = "3"
	next.ArtifactSignature = signing.Sign(f.priv, next.Product, next.TargetVersion, next.ArtifactSHA256)
	next.Deadline = time.Now().UTC().Add(time.Hour)
	claims := p.NextOperationClaims{Protocol: p.NextOperationProtocol, AuthorizationID: "qualification-next-auth", PreviousBinding: j.Binding, PreviousGeneration: j.Generation, PreviousOutcome: p.PredecessorRestored, NextBinding: next, IssuedAt: time.Now().UTC().Add(-time.Second), ExpiresAt: time.Now().UTC().Add(time.Minute)}
	raw, _ := json.Marshal(claims)
	f.c.Mode = "next-operation"
	f.c.Authorization = p.RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(f.observer, append([]byte(p.NextOperationDomain), raw...)))}
	put(t, f.config, f.c)
	f.fault(t, "authorize-next-operation", "after", "lost-response")
	f.run(t, false)
	still, _ := os.ReadFile(filepath.Join(f.c.Directory, "operation.json"))
	if string(still) != string(old) {
		t.Fatal("journal changed on lost authorization")
	}
	f.run(t, true)
	j, e = p.ReadJournal(f.c.Directory)
	if e != nil || j.Binding.OperationID != next.OperationID || j.Phase != "intent" {
		t.Fatal(j, e)
	}
	matches, _ := filepath.Glob(filepath.Join(f.c.Directory, "archive", "*", "operation.json"))
	if len(matches) != 1 {
		t.Fatal(matches)
	}
	archived, _ := os.ReadFile(matches[0])
	if string(archived) != string(old) {
		t.Fatal("archive evidence changed")
	}
}

func prepareNext(t *testing.T, f *fixture) []byte {
	t.Helper()
	f.run(t, true)
	old, _ := os.ReadFile(filepath.Join(f.c.Directory, "operation.json"))
	j, e := p.ReadJournal(f.c.Directory)
	if e != nil {
		t.Fatal(e)
	}
	next := j.Binding
	next.OperationID = "qualification-next-operation"
	next.TargetVersion = "3"
	next.ArtifactSignature = signing.Sign(f.priv, next.Product, next.TargetVersion, next.ArtifactSHA256)
	next.Deadline = time.Now().UTC().Add(time.Hour)
	claims := p.NextOperationClaims{Protocol: p.NextOperationProtocol, AuthorizationID: "qualification-next-auth", PreviousBinding: j.Binding, PreviousGeneration: j.Generation, PreviousOutcome: p.PredecessorRestored, NextBinding: next, IssuedAt: time.Now().UTC().Add(-time.Second), ExpiresAt: time.Now().UTC().Add(time.Minute)}
	raw, _ := json.Marshal(claims)
	f.c.Mode = "next-operation"
	f.c.Authorization = p.RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(f.observer, append([]byte(p.NextOperationDomain), raw...)))}
	put(t, f.config, f.c)
	return old
}
func TestRealProcessArchivalCrashCheckpoints(t *testing.T) {
	for _, point := range []string{"prepared", "recovery-archived", "next-intent-written"} {
		t.Run(point, func(t *testing.T) {
			t.Parallel()
			f := setup(t, "recovery-v2", p.PredecessorRestored)
			old := prepareNext(t, &f)
			f.c.FaultCheckpoint = point
			put(t, f.config, f.c)
			f.run(t, false)
			f.run(t, true)
			j, e := p.ReadJournal(f.c.Directory)
			if e != nil || j.Phase != "intent" || j.Binding.OperationID != "qualification-next-operation" {
				t.Fatal(j, e)
			}
			files, _ := filepath.Glob(filepath.Join(f.c.Directory, "archive", "*", "operation.json"))
			if len(files) != 1 {
				t.Fatal(files)
			}
			archived, _ := os.ReadFile(files[0])
			if string(archived) != string(old) {
				t.Fatal("evidence lost")
			}
		})
	}
}
func TestUnsupportedAndNotReadyRefuseMutation(t *testing.T) {
	for _, mode := range []string{"maintenance-v1", "recovery-v2", "readiness"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t, mode, p.TargetInstalled)
			root := filepath.Join(f.root, "reference")
			os.MkdirAll(root, 0700)
			os.WriteFile(filepath.Join(root, "unsupported"), []byte("true"), 0600)
			f.run(t, false)
			for _, action := range f.calls(t) {
				if action != "capabilities" && action != "readiness" {
					t.Fatal("mutated despite unsupported/not-ready", action)
				}
			}
		})
	}
}
