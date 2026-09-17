package updatersim

import (
	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"testing"
	"time"
)

func TestInstallationClassSurvivesRelayAndRoleChange(t *testing.T) {
	for _, kind := range []string{"normal", "pod-active", "pod-stby", "pod-observer"} {
		cfg := newSimulatorTestConfig(t, "https://example.invalid", ModeObserve)
		p := &cfg.Products[0]
		p.ServerType = kind
		if kind != "normal" {
			p.PodID = "pod"
			p.NodeID = "1"
			if kind == "pod-observer" {
				p.NodeID = "witness"
			}
		}
		sim, e := NewSimulator(cfg, NoopExecutor{}, discardLogger())
		if e != nil {
			t.Fatal(e)
		}
		hb := sim.buildHeartbeat()
		want := "pod"
		if kind == "normal" {
			want = "normal"
		}
		if hb.Installation == nil || hb.Installation.Kind != want {
			t.Fatal(hb.Installation)
		}
		if kind == "pod-active" || kind == "pod-stby" {
			p.ServerType = "pod-stby"
			if kind == "pod-stby" {
				p.ServerType = "pod-active"
			}
			if err := rememberSiemCoreInstallation(cfg, sim.state); err != nil {
				t.Fatal(err)
			}
			after := sim.buildHeartbeat()
			if after.Installation == nil || *after.Installation != *hb.Installation {
				t.Fatal("role transition changed installation class")
			}
		}
		relay := newDecommissionTestRelay(t, t.TempDir(), false)
		relay.children["node"] = &childState{Heartbeat: hb, LastSeen: time.Now()}
		full := relay.ChildrenReport()
		if len(full) != 1 || full[0].Installation == nil || *full[0].Installation != *hb.Installation {
			t.Fatal("full rollup lost identity")
		}
		delta := childReport(hb, "online", time.Now(), "127.0.0.1")
		if delta.Installation == nil || *delta.Installation != *hb.Installation {
			t.Fatal("delta lost identity")
		}
	}
}
func TestLegacyInstallationIdentityIsNotGuessed(t *testing.T) {
	s := Simulator{state: &State{}, config: &Config{Products: []ProductConfig{{Name: "siemcore"}}}}
	if s.installationIdentity() != nil {
		t.Fatal("unreported legacy type guessed")
	}
	for _, bad := range []platformtypes.InstallationIdentity{{Kind: "active"}, {Kind: "normal", PodID: "pod"}, {Kind: "pod", PodID: "pod", NodeID: "bad"}} {
		if bad.Validate() == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}
