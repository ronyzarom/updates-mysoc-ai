package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/catalog"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/licensing"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// handleListProducts returns the canonical product-tier catalog
// (mysoc > siemcore > swf) used by dashboard dropdowns and agent config.
func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tiers": catalog.Tiers(),
	})
}

// validateHierarchy checks a self-reported (tier, parent) pair on ingest.
//
// The tier is optional for backward compatibility: an empty tier passes. When a
// tier is supplied it must be canonical. Parent linkage is validated best-effort:
// if the declared parent already exists and carries a tier, it must be exactly
// one rank above; unknown parents are accepted and reconciled later as orphans.
// Returns a human-readable message and false when the payload should be rejected.
func (s *Server) validateHierarchy(ctx context.Context, tier, parentInstanceID string) (string, bool) {
	if tier == "" {
		return "", true
	}
	if !catalog.IsValidTier(tier) {
		return fmt.Sprintf("invalid product_tier %q (must be mysoc, siemcore, or swf)", tier), false
	}

	expectedParent, _ := catalog.ExpectedParentTier(tier)
	if parentInstanceID == "" {
		// A root (mysoc) legitimately has no parent. Deeper tiers may enroll
		// before their parent, so a missing parent is accepted (orphan) rather
		// than rejected.
		return "", true
	}
	if expectedParent == "" {
		// Root tier must not point at a parent.
		return fmt.Sprintf("product_tier %q is a root and must not declare parent_instance_id", tier), false
	}

	repo := licensing.NewInstanceRepository(s.db)
	parent, err := repo.GetByInstanceID(ctx, parentInstanceID)
	if err != nil || parent == nil || parent.ProductTier == "" {
		// Parent not enrolled yet (or untyped) — accept and reconcile later.
		return "", true
	}
	if !catalog.ParentTierMatches(tier, parent.ProductTier) {
		return fmt.Sprintf("parent %q is tier %q but a %q node requires a %q parent",
			parentInstanceID, parent.ProductTier, tier, expectedParent), false
	}
	return "", true
}

// ---- Tree assembly ----

type treeNode struct {
	ID               string      `json:"id"`
	InstanceID       string      `json:"instance_id"`
	DisplayName      string      `json:"display_name,omitempty"`
	Hostname         string      `json:"hostname,omitempty"`
	InstanceType     string      `json:"instance_type,omitempty"`
	ProductTier      string      `json:"product_tier,omitempty"`
	ParentInstanceID string      `json:"parent_instance_id,omitempty"`
	Status           string      `json:"status"`
	LastHeartbeat    *time.Time  `json:"last_heartbeat,omitempty"`
	Orphan           bool        `json:"orphan,omitempty"` // parent declared but not found in this customer
	Children         []*treeNode `json:"children"`
}

type treeCustomer struct {
	LicenseID    string      `json:"license_id,omitempty"`
	LicenseKey   string      `json:"license_key,omitempty"` // masked
	CustomerID   string      `json:"customer_id,omitempty"`
	CustomerName string      `json:"customer_name"`
	TotalNodes   int         `json:"total_nodes"`
	Roots        []*treeNode `json:"roots"`
}

type instanceTreeResponse struct {
	Customers []*treeCustomer `json:"customers"`
}

// handleInstanceTree assembles the fleet into a per-customer tier tree:
// license -> mysoc -> siemcore -> swf. Instances whose declared parent is not
// present in the same customer are surfaced as orphan roots (flagged), and
// instances with no bound license fall into an "Unlicensed / unbound" bucket.
func (s *Server) handleInstanceTree(w http.ResponseWriter, r *http.Request) {
	instanceRepo := licensing.NewInstanceRepository(s.db)
	instances, err := instanceRepo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list instances")
		return
	}

	licenseSvc := licensing.NewService(s.db)
	licenses, err := licenseSvc.ListLicenses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list licenses")
		return
	}
	licenseByID := make(map[string]types.License, len(licenses))
	for _, l := range licenses {
		licenseByID[l.ID] = l
	}

	const unlicensedKey = ""

	// Bucket instances by their bound license (or the unlicensed bucket).
	bucketOrder := []string{}
	buckets := map[string][]types.Instance{}
	for _, inst := range instances {
		key := inst.LicenseID
		if _, known := licenseByID[key]; !known {
			key = unlicensedKey
		}
		if _, seen := buckets[key]; !seen {
			bucketOrder = append(bucketOrder, key)
		}
		buckets[key] = append(buckets[key], inst)
	}

	resp := instanceTreeResponse{Customers: []*treeCustomer{}}
	for _, key := range bucketOrder {
		group := buckets[key]
		customer := &treeCustomer{TotalNodes: len(group)}
		if lic, ok := licenseByID[key]; ok {
			customer.LicenseID = lic.ID
			customer.LicenseKey = maskLicenseKey(lic.LicenseKey)
			customer.CustomerID = lic.CustomerID
			customer.CustomerName = lic.CustomerName
			if customer.CustomerName == "" {
				customer.CustomerName = "Unnamed customer"
			}
		} else {
			customer.CustomerName = "Unlicensed / unbound"
		}
		customer.Roots = assembleTree(group)
		resp.Customers = append(resp.Customers, customer)
	}

	// Stable customer ordering: named customers first (by name), unlicensed last.
	sort.SliceStable(resp.Customers, func(i, j int) bool {
		ci, cj := resp.Customers[i], resp.Customers[j]
		if (ci.LicenseID == "") != (cj.LicenseID == "") {
			return ci.LicenseID != "" // licensed groups before the unlicensed bucket
		}
		return strings.ToLower(ci.CustomerName) < strings.ToLower(cj.CustomerName)
	})

	writeJSON(w, http.StatusOK, resp)
}

// assembleTree links a single customer's instances into a tier tree. A node is a
// root when it declares no parent (a mysoc) or when its declared parent is not in
// this customer's set (an orphan, flagged for the UI).
func assembleTree(group []types.Instance) []*treeNode {
	nodeByInstanceID := make(map[string]*treeNode, len(group))
	nodes := make([]*treeNode, 0, len(group))
	for i := range group {
		inst := group[i]
		n := &treeNode{
			ID:               inst.ID,
			InstanceID:       inst.InstanceID,
			DisplayName:      inst.DisplayName,
			Hostname:         inst.Hostname,
			InstanceType:     inst.InstanceType,
			ProductTier:      inst.ProductTier,
			ParentInstanceID: inst.ParentInstanceID,
			Status:           inst.Status,
			LastHeartbeat:    inst.LastHeartbeat,
			Children:         []*treeNode{},
		}
		nodes = append(nodes, n)
		nodeByInstanceID[inst.InstanceID] = n
	}

	roots := []*treeNode{}
	for _, n := range nodes {
		if n.ParentInstanceID == "" {
			roots = append(roots, n)
			continue
		}
		if parent, ok := nodeByInstanceID[n.ParentInstanceID]; ok && parent != n {
			parent.Children = append(parent.Children, n)
			continue
		}
		// Declared a parent we can't resolve within this customer.
		n.Orphan = true
		roots = append(roots, n)
	}

	sortNodes(roots)
	for _, n := range nodes {
		sortNodes(n.Children)
	}
	return roots
}

// sortNodes orders siblings by tier rank (shallowest first) then instance_id.
func sortNodes(nodes []*treeNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		ri, rj := tierRank(nodes[i].ProductTier), tierRank(nodes[j].ProductTier)
		if ri != rj {
			return ri < rj
		}
		return nodes[i].InstanceID < nodes[j].InstanceID
	})
}

func tierRank(tier string) int {
	if t, ok := catalog.Lookup(tier); ok {
		return t.Rank
	}
	return 99 // unknown/empty tiers sort last
}

// maskLicenseKey reveals only the first and last segments of a license key so an
// authenticated (non-admin) dashboard user can identify a customer without
// exposing the full credential.
func maskLicenseKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "…" + key[len(key)-4:]
}
