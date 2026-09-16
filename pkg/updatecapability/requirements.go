// Package updatecapability implements fail-closed offer eligibility, not artifact trust.
package updatecapability

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Requirements struct {
	Scope             string   `json:"scope"`
	Capabilities      []string `json:"capabilities"`
	MinUpdaterVersion string   `json:"min_updater_version"`
}

var versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,3}$`)

func version(s string) ([4]uint64, error) {
	var v [4]uint64
	if !versionPattern.MatchString(s) {
		return v, fmt.Errorf("invalid updater version")
	}
	for i, n := range strings.Split(s, ".") {
		x, e := strconv.ParseUint(n, 10, 64)
		if e != nil {
			return v, e
		}
		v[i] = x
	}
	return v, nil
}
func (r *Requirements) Validate() error {
	if r == nil {
		return nil
	}
	if r.Scope != "pod-node" || len(r.Capabilities) == 0 || len(r.Capabilities) > 2 {
		return fmt.Errorf("invalid updater requirements")
	}
	seen := map[string]bool{}
	for _, c := range r.Capabilities {
		if (c != "pod-maintenance-v1" && c != "pod-maintenance-recovery-v2") || seen[c] {
			return fmt.Errorf("unknown or duplicate required capability")
		}
		seen[c] = true
	}
	if !seen["pod-maintenance-v1"] {
		return fmt.Errorf("maintenance-v1 required")
	}
	_, e := version(r.MinUpdaterVersion)
	return e
}
func (r *Requirements) Check(role, updaterVersion string, capabilities []string) error {
	if r == nil {
		return nil
	}
	if e := r.Validate(); e != nil {
		return e
	}
	if role == "normal" {
		return nil
	}
	if role != "pod-node" {
		return fmt.Errorf("explicit supported deployment role required")
	}
	have, e := version(updaterVersion)
	if e != nil {
		return e
	}
	want, _ := version(r.MinUpdaterVersion)
	for i := range have {
		if have[i] < want[i] {
			return fmt.Errorf("updater version below required minimum")
		}
		if have[i] > want[i] {
			break
		}
	}
	for _, required := range r.Capabilities {
		found := false
		for _, c := range capabilities {
			if c == required {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("missing required capability %s", required)
		}
	}
	return nil
}
