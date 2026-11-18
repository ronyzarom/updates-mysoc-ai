package releases

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// Retention deletes releases that no longer serve the fleet. Per product it
// keeps the newest ProductionKeep releases that reached the production ring
// and everything above the production head (still rolling out). It deletes
// older production releases and releases superseded before reaching
// production. A ring head, a version any device reports, a recent upload, an
// unparsable version, and every dual-artifact qualification release are kept.

const (
	ReasonOldProduction   = "old-production"
	ReasonNeverProduction = "never-reached-production"

	productionGroup = "production"
)

var retentionRings = []string{"alpha", "beta", "stable", "production"}

// RetentionPolicy configures one retention pass. ProductionKeep <= 0 disables
// retention entirely.
type RetentionPolicy struct {
	ProductionKeep int
	MinAge         time.Duration
	MaxPerRun      int
	// QualificationPrefix is the dual-artifact channel prefix; releases in that
	// namespace are never touched (the "dual-alpha-" form is always honored).
	QualificationPrefix string
}

// Installed is the set of versions the fleet still reports, the input that
// keeps retention from deleting under a running device.
type Installed struct {
	Devices   int
	byProduct map[string]map[string]bool
	updater   map[string]bool
	anyProd   map[string]bool
}

func NewInstalled(devices int) *Installed {
	return &Installed{
		Devices:   devices,
		byProduct: map[string]map[string]bool{},
		updater:   map[string]bool{},
		anyProd:   map[string]bool{},
	}
}

// Add records a product version a device reports installed.
func (in *Installed) Add(product, version string) {
	if product == "" || version == "" {
		return
	}
	if in.byProduct[product] == nil {
		in.byProduct[product] = map[string]bool{}
	}
	in.byProduct[product][versionKey(version)] = true
}

// AddUpdater records an updater version; it protects that version for every
// updater-* product because the heartbeat does not name the platform.
func (in *Installed) AddUpdater(version string) {
	if version != "" {
		in.updater[versionKey(version)] = true
	}
}

// AddAnyProduct records a version whose product is unknown (the last update
// attempt columns); it protects that version for every product.
func (in *Installed) AddAnyProduct(version string) {
	if version != "" {
		in.anyProd[versionKey(version)] = true
	}
}

// AddReported records one row of licensing.InstanceRepository.InstalledVersions.
func (in *Installed) AddReported(kind, product, version string) {
	switch kind {
	case "product":
		in.Add(product, version)
	case "updater":
		in.AddUpdater(version)
	default:
		in.AddAnyProduct(version)
	}
}

func (in *Installed) has(product, version string) bool {
	key := versionKey(version)
	if in.anyProd[key] || in.byProduct[product][key] {
		return true
	}
	return isSelfUpdateProduct(product) && in.updater[key]
}

// versionKey normalizes "v1.2" and "1.2.0.0" to one key; unparsable versions
// match only themselves.
func versionKey(version string) string {
	if p, ok := parseVersion(version); ok {
		return fmt.Sprintf("%d.%d.%d.%d", p[0], p[1], p[2], p[3])
	}
	return version
}

// compareVersions orders two parsable versions: -1, 0 or 1.
func compareVersions(a, b [4]int) int {
	for i := 0; i < 4; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// RetentionCandidate is a release a retention pass deletes, and why.
type RetentionCandidate struct {
	Release types.Release
	Reason  string
}

func (p RetentionPolicy) isQualification(r *types.Release) bool {
	if r.Manifest.ArtifactKind != "" || len(r.Manifest.ArtifactVariants) > 0 {
		return true
	}
	if strings.HasPrefix(r.Channel, QualificationChannelPrefix) {
		return true
	}
	return p.QualificationPrefix != "" && strings.HasPrefix(r.Channel, p.QualificationPrefix)
}

func inGroup(r *types.Release, group string) bool {
	for _, g := range r.TargetGroups {
		if g == group {
			return true
		}
	}
	return false
}

// ringHeads returns the IDs of the releases the update check currently offers
// to each ring, using the same selection as HighestReleaseForGroup.
func ringHeads(all []types.Release) map[string]bool {
	byProduct := map[string][]types.Release{}
	for _, r := range all {
		if r.Manifest.ArtifactKind == "" {
			byProduct[r.ProductName] = append(byProduct[r.ProductName], r)
		}
	}
	heads := map[string]bool{}
	for _, releases := range byProduct {
		for _, ring := range retentionRings {
			var offered []types.Release
			for _, r := range releases {
				if inGroup(&r, ring) {
					offered = append(offered, r)
				}
			}
			if head := findHighestVersion(offered); head != nil {
				heads[head.ID] = true
			}
		}
	}
	return heads
}

// PlanRetention selects the releases a retention pass deletes. It is pure.
func PlanRetention(all []types.Release, installed *Installed, now time.Time, p RetentionPolicy) []RetentionCandidate {
	if p.ProductionKeep <= 0 || installed == nil {
		return nil
	}
	heads := ringHeads(all)

	type ranked struct {
		release types.Release
		version [4]int
	}
	byProduct := map[string][]ranked{}
	for _, r := range all {
		if p.isQualification(&r) {
			continue
		}
		v, ok := parseVersion(r.Version)
		if !ok {
			continue
		}
		byProduct[r.ProductName] = append(byProduct[r.ProductName], ranked{r, v})
	}

	var out []RetentionCandidate
	for _, releases := range byProduct {
		sort.SliceStable(releases, func(i, j int) bool {
			return compareVersions(releases[i].version, releases[j].version) > 0
		})
		var head [4]int
		hasProduction := false
		keptProduction := 0
		keep := map[string]bool{}
		for _, r := range releases {
			if !inGroup(&r.release, productionGroup) {
				continue
			}
			if !hasProduction {
				head, hasProduction = r.version, true
			}
			if keptProduction < p.ProductionKeep {
				keep[r.release.ID] = true
				keptProduction++
			}
		}
		if !hasProduction {
			continue
		}
		for _, r := range releases {
			rel := r.release
			if keep[rel.ID] || heads[rel.ID] || installed.has(rel.ProductName, rel.Version) {
				continue
			}
			if uploaded := latest(rel.CreatedAt, rel.ReleasedAt); now.Sub(uploaded) < p.MinAge {
				continue
			}
			switch {
			case inGroup(&rel, productionGroup):
				out = append(out, RetentionCandidate{rel, ReasonOldProduction})
			case compareVersions(r.version, head) < 0:
				out = append(out, RetentionCandidate{rel, ReasonNeverProduction})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Release, out[j].Release
		if a.ProductName != b.ProductName {
			return a.ProductName < b.ProductName
		}
		va, _ := parseVersion(a.Version)
		vb, _ := parseVersion(b.Version)
		return compareVersions(va, vb) < 0
	})
	if p.MaxPerRun > 0 && len(out) > p.MaxPerRun {
		out = out[:p.MaxPerRun]
	}
	return out
}

func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// RetentionResult summarizes one retention pass.
type RetentionResult struct {
	Scanned int
	Deleted []RetentionCandidate
	Failed  int
}

// PruneReleases runs one retention pass. It deletes nothing when the fleet's
// installed versions cannot be loaded or no device is known, since an empty
// fleet would make every old version look unused.
func (s *Service) PruneReleases(ctx context.Context, p RetentionPolicy, loadInstalled func(context.Context) (*Installed, error), now time.Time) (RetentionResult, error) {
	var res RetentionResult
	if p.ProductionKeep <= 0 {
		return res, nil
	}
	installed, err := loadInstalled(ctx)
	if err != nil {
		return res, fmt.Errorf("load installed versions: %w", err)
	}
	if installed == nil || installed.Devices == 0 {
		return res, fmt.Errorf("no devices known; refusing to prune")
	}
	all, err := s.repo.List(ctx)
	if err != nil {
		return res, fmt.Errorf("list releases: %w", err)
	}
	res.Scanned = len(all)
	for _, c := range PlanRetention(all, installed, now, p) {
		if err := s.DeleteRelease(ctx, c.Release.ID); err != nil {
			res.Failed++
			log.Printf("WARNING release-retention: %s %s: delete failed: %v", c.Release.ProductName, c.Release.Version, err)
			continue
		}
		res.Deleted = append(res.Deleted, c)
		log.Printf("release-retention: deleted %s %s (%s)", c.Release.ProductName, c.Release.Version, c.Reason)
	}
	return res, nil
}

// RunRetention runs PruneReleases after firstDelay and then every interval
// until ctx is cancelled.
func (s *Service) RunRetention(ctx context.Context, p RetentionPolicy, interval, firstDelay time.Duration, loadInstalled func(context.Context) (*Installed, error)) {
	if p.ProductionKeep <= 0 || interval <= 0 {
		log.Printf("release-retention: disabled")
		return
	}
	log.Printf("release-retention: keeping %d production releases per product, every %s", p.ProductionKeep, interval)
	timer := time.NewTimer(firstDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		res, err := s.PruneReleases(ctx, p, loadInstalled, time.Now())
		if err != nil {
			log.Printf("WARNING release-retention: pass skipped: %v", err)
		} else {
			log.Printf("release-retention: pass done: scanned %d, deleted %d, failed %d", res.Scanned, len(res.Deleted), res.Failed)
		}
		timer.Reset(interval)
	}
}
