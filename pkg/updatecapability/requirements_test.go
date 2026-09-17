package updatecapability

import "testing"

func TestRoleVersionAndCapabilities(t *testing.T) {
	r := &Requirements{Scope: "pod-node", Capabilities: []string{"pod-maintenance-v1", "pod-maintenance-recovery-v2"}, MinUpdaterVersion: "1.16.1.25"}
	cases := []struct {
		role, version string
		caps          []string
		ok            bool
	}{{"", "1.16.1.25", r.Capabilities, false}, {"witness", "1.16.1.25", r.Capabilities, false}, {"normal", "1.0", nil, true}, {"pod-node", "1.16.1.24", r.Capabilities, false}, {"pod-node", "1.16.1.25", nil, false}, {"pod-node", "1.16.1.25", r.Capabilities, true}, {"pod-node", "1.16.1.26", r.Capabilities, true}, {"pod-node", "1.16.1.25-junk", r.Capabilities, false}}
	for _, c := range cases {
		if got := r.Check(c.role, c.version, c.caps) == nil; got != c.ok {
			t.Errorf("%+v got %v", c, got)
		}
	}
	var legacy *Requirements
	if legacy.Check("", "", nil) != nil {
		t.Fatal("legacy changed")
	}
}

func TestAckV2RequirementCannotUseLegacyCapability(t *testing.T) {
	r := &Requirements{Scope: "pod-node", Capabilities: []string{"pod-maintenance-ack-v2"}, MinUpdaterVersion: "1.16.1.26"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Check("pod-node", "1.16.1.26", []string{"pod-maintenance-v1"}) == nil {
		t.Fatal("v1 accepted ACK-v2 artifact")
	}
	if err := r.Check("pod-node", "1.16.1.26", []string{"pod-maintenance-ack-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Check("normal", "1.16.1.25", nil); err != nil {
		t.Fatal("normal behavior changed", err)
	}
}
