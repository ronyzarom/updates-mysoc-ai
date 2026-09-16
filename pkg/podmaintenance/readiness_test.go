package podmaintenance

import (
	"testing"
	"time"
)

func TestReadinessMustProveCompleteFreshIdentity(t *testing.T) {
	now := time.Now().UTC()
	q := ReadinessRequest{Protocol: ReadinessProtocol, PodID: "pod", NodeID: "1", UpdaterID: "updater"}
	r := ReadinessResponse{Protocol: q.Protocol, PodID: q.PodID, NodeID: q.NodeID, UpdaterID: q.UpdaterID, Capabilities: []string{Protocol, RecoveryProtocol}, Ready: true, ObserverVerified: true, CredentialsReady: true, LifecycleReady: true, IssuedAt: now, ValidUntil: now.Add(time.Minute)}
	if e := r.Validate(q, now); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"expired", "overlong", "wrong-node", "credentials", "lifecycle", "observer", "future"} {
		bad := r
		switch kind {
		case "expired":
			bad.ValidUntil = now
		case "overlong":
			bad.ValidUntil = now.Add(61 * time.Second)
		case "wrong-node":
			bad.NodeID = "2"
		case "credentials":
			bad.CredentialsReady = false
		case "lifecycle":
			bad.LifecycleReady = false
		case "observer":
			bad.ObserverVerified = false
		case "future":
			bad.IssuedAt = now.Add(time.Second)
		}
		if bad.Validate(q, now) == nil {
			t.Fatal(kind)
		}
	}
}
