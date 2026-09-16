package updatersim

import (
	"context"
	"encoding/json"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/updatecapability"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPodCapabilityAdvertisementDefaultsOffAndRequirementsFailClosed(t *testing.T) {
	cfg := &Config{}
	cfg.Instance.ID = "updater"
	cfg.Instance.UpdaterVersion = "1.16.1.25"
	cfg.Products = []ProductConfig{{Name: "siemcore", CurrentVersion: "1"}}
	cfg.Simulation.Filesystem.PodMaintenance = &PodMaintenanceConfig{PodID: "pod", NodeID: "1", AdapterCommand: []string{"/must-not-execute"}}
	s := &Simulator{config: cfg}
	role, caps, e := s.podCapabilities(context.Background(), "siemcore")
	if e != nil || role != "pod-node" || len(caps) != 0 {
		t.Fatal(role, caps, e)
	}
	offer := &UpdateOffer{Product: "siemcore", UpdaterRequirements: &updatecapability.Requirements{Scope: "pod-node", MinUpdaterVersion: "1.16.1.25", Capabilities: []string{"pod-maintenance-v1"}}}
	if s.verifyOfferRequirements(context.Background(), offer) == nil {
		t.Fatal("disabled capability accepted")
	}
	cfg.Simulation.Filesystem.PodMaintenance = nil
	cfg.Products[0].DeploymentRole = "normal"
	if e = s.verifyOfferRequirements(context.Background(), offer); e != nil {
		t.Fatal("normal behavior changed", e)
	}
	cfg.Products[0].DeploymentRole = "witness"
	if s.verifyOfferRequirements(context.Background(), offer) == nil {
		t.Fatal("witness treated as normal")
	}
}
func TestRequirementWireRoundTrip(t *testing.T) {
	required := &updatecapability.Requirements{Scope: "pod-node", MinUpdaterVersion: "1.16.1.25", Capabilities: []string{"pod-maintenance-v1"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q UpdateCheckRequest
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			t.Error(e)
		}
		if q.DeploymentRole != "pod-node" {
			t.Error("lost role")
		}
		json.NewEncoder(w).Encode(UpdateCheckResponse{UpdateAvailable: true, LatestVersion: "2", UpdaterRequirements: required})
	}))
	defer server.Close()
	client, e := NewClient(ServerConfig{URL: server.URL, MaxResponseBytes: 1 << 20})
	if e != nil {
		t.Fatal(e)
	}
	offer, e := client.CheckUpdate(context.Background(), "siemcore", UpdateCheckRequest{DeploymentRole: "pod-node"})
	if e != nil || offer.UpdaterRequirements == nil || offer.UpdaterRequirements.MinUpdaterVersion != required.MinUpdaterVersion {
		t.Fatal(offer, e)
	}
}
