package releases

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

var (
	retentionNow = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	allRings     = []string{"alpha", "beta", "stable", "production"}
)

func rel(product, version string, groups ...string) types.Release {
	return types.Release{
		ID:           product + "@" + version,
		ProductName:  product,
		Version:      version,
		Channel:      "stable",
		TargetGroups: groups,
		CreatedAt:    retentionNow.Add(-30 * 24 * time.Hour),
		ReleasedAt:   retentionNow.Add(-30 * 24 * time.Hour),
	}
}

func planned(all []types.Release, installed *Installed, p RetentionPolicy) map[string]string {
	out := map[string]string{}
	for _, c := range PlanRetention(all, installed, retentionNow, p) {
		out[c.Release.ID] = c.Reason
	}
	return out
}

func keys(m map[string]string) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func policy() RetentionPolicy {
	return RetentionPolicy{ProductionKeep: 5, MinAge: 7 * 24 * time.Hour, MaxPerRun: 100, QualificationPrefix: "dual-alpha-"}
}

func TestPlanRetentionKeepsFiveProductionVersions(t *testing.T) {
	var all []types.Release
	for _, v := range []string{"1.0.0.1", "1.0.1.1", "1.0.2.1", "1.0.3.1", "1.0.4.1", "1.0.5.1", "1.0.6.1"} {
		all = append(all, rel("mysoc", v, allRings...))
	}
	got := planned(all, NewInstalled(1), policy())
	if keys(got) != "mysoc@1.0.0.1,mysoc@1.0.1.1" {
		t.Fatalf("want the two oldest production releases deleted, got %v", got)
	}
	for id, reason := range got {
		if reason != ReasonOldProduction {
			t.Fatalf("%s: reason %q", id, reason)
		}
	}
}

func TestPlanRetentionNeverReachedProduction(t *testing.T) {
	all := []types.Release{
		rel("siemcore", "3.3.190.1", "alpha"),
		rel("siemcore", "3.3.191.1", "alpha", "beta"),
		rel("siemcore", "3.3.192.1", "alpha", "beta", "stable"),
		rel("siemcore", "3.3.193.1", allRings...),
		rel("siemcore", "3.3.194.1", "alpha", "beta"),
		rel("siemcore", "3.3.195.1", "alpha"),
	}
	got := planned(all, NewInstalled(1), policy())
	if keys(got) != "siemcore@3.3.190.1,siemcore@3.3.191.1,siemcore@3.3.192.1" {
		t.Fatalf("want only the never-production releases below the production head, got %v", got)
	}
	for id, reason := range got {
		if reason != ReasonNeverProduction {
			t.Fatalf("%s: reason %q", id, reason)
		}
	}
}

func TestPlanRetentionProductWithoutProductionKeepsEverything(t *testing.T) {
	all := []types.Release{rel("swf", "2.3.0.1", "alpha"), rel("swf", "2.3.0.2", "alpha"), rel("swf", "2.3.0.3", "alpha", "beta")}
	if got := planned(all, NewInstalled(1), policy()); len(got) != 0 {
		t.Fatalf("no production release: nothing may be deleted, got %v", got)
	}
}

func TestPlanRetentionProtections(t *testing.T) {
	base := func() []types.Release {
		all := []types.Release{rel("mysoc", "1.0.0.1", "alpha")}
		for _, v := range []string{"1.1.0.1", "1.2.0.1", "1.3.0.1", "1.4.0.1", "1.5.0.1"} {
			all = append(all, rel("mysoc", v, allRings...))
		}
		return all
	}
	cases := []struct {
		name    string
		mutate  func([]types.Release) []types.Release
		install func(*Installed)
	}{
		{"installed on a device", nil, func(in *Installed) { in.Add("mysoc", "1.0.0.1") }},
		{"installed, normalized version", nil, func(in *Installed) { in.Add("mysoc", "v1.0.0.1") }},
		{"last update attempt, product unknown", nil, func(in *Installed) { in.AddAnyProduct("1.0.0.1") }},
		{"recent upload", func(all []types.Release) []types.Release {
			all[0].CreatedAt = retentionNow.Add(-24 * time.Hour)
			return all
		}, nil},
		{"recently re-released", func(all []types.Release) []types.Release {
			all[0].ReleasedAt = retentionNow.Add(-24 * time.Hour)
			return all
		}, nil},
		{"unparsable version", func(all []types.Release) []types.Release {
			all[0].Version, all[0].ID = "legacy-build", "mysoc@legacy-build"
			return all
		}, nil},
		{"dual-alpha qualification channel", func(all []types.Release) []types.Release {
			all[0].Channel = "dual-alpha-mysoc"
			return all
		}, nil},
		{"independent artifact kind", func(all []types.Release) []types.Release {
			all[0].Manifest.ArtifactKind = "bootstrap"
			return all
		}, nil},
		{"paired variants", func(all []types.Release) []types.Release {
			all[0].Manifest.ArtifactVariants = []types.Artifact{{Name: "update.bin"}}
			return all
		}, nil},
	}
	if got := planned(base(), NewInstalled(1), policy()); keys(got) != "mysoc@1.0.0.1" {
		t.Fatalf("baseline: want mysoc@1.0.0.1 deleted, got %v", got)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			all := base()
			if tc.mutate != nil {
				all = tc.mutate(all)
			}
			installed := NewInstalled(1)
			if tc.install != nil {
				tc.install(installed)
			}
			if got := planned(all, installed, policy()); len(got) != 0 {
				t.Fatalf("protected release deleted: %v", got)
			}
		})
	}
}

func TestPlanRetentionKeepsRingHeads(t *testing.T) {
	// Production head is 2.0.0.1, but the beta ring is still offered 1.0.0.1
	// because the newer releases were tagged production-only.
	all := []types.Release{rel("mysoc", "1.0.0.1", "alpha", "beta"), rel("mysoc", "0.9.0.1", "alpha", "beta")}
	for _, v := range []string{"2.0.0.1", "2.1.0.1"} {
		all = append(all, rel("mysoc", v, "production"))
	}
	got := planned(all, NewInstalled(1), policy())
	if keys(got) != "mysoc@0.9.0.1" {
		t.Fatalf("the beta ring head must survive, got %v", got)
	}
}

func TestPlanRetentionUpdaterVersionProtectsUpdaterProducts(t *testing.T) {
	var all []types.Release
	for _, product := range []string{"updater-linux-amd64", "mysoc"} {
		all = append(all, rel(product, "1.10.0.1", "alpha"))
		for _, v := range []string{"1.11.0.1", "1.12.0.1", "1.13.0.1", "1.14.0.1", "1.15.0.1"} {
			all = append(all, rel(product, v, allRings...))
		}
	}
	installed := NewInstalled(1)
	installed.AddUpdater("1.10.0.1")
	if got := planned(all, installed, policy()); keys(got) != "mysoc@1.10.0.1" {
		t.Fatalf("updater version must protect updater-* only, got %v", got)
	}
}

func TestPlanRetentionCapAndDisable(t *testing.T) {
	var all []types.Release
	for _, v := range []string{"1.0.0.1", "1.0.0.2", "1.0.0.3", "1.0.0.4", "1.0.0.5", "1.0.0.6", "1.0.0.7", "1.0.0.8"} {
		all = append(all, rel("mysoc", v, allRings...))
	}
	p := policy()
	p.MaxPerRun = 2
	if got := planned(all, NewInstalled(1), p); keys(got) != "mysoc@1.0.0.1,mysoc@1.0.0.2" {
		t.Fatalf("cap must keep the oldest first, got %v", got)
	}
	p.ProductionKeep = 0
	if got := planned(all, NewInstalled(1), p); len(got) != 0 {
		t.Fatalf("ProductionKeep 0 disables retention, got %v", got)
	}
}
