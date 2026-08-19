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
	CustomerID       string      `json:"customer_id,omitempty"`
	Version          string      `json:"version,omitempty"` // primary product version from the latest report
	Status           string      `json:"status"`
	LastHeartbeat    *time.Time  `json:"last_heartbeat,omitempty"`
	ReportedVia      string      `json:"reported_via,omitempty"` // relay that reported this node (empty = direct)
	ReportedAt       *time.Time  `json:"reported_at,omitempty"`
	Orphan           bool        `json:"orphan,omitempty"` // parent declared but not enrolled anywhere in the fleet
	Children         []*treeNode `json:"children"`
}

type treeCustomer struct {
	LicenseID    string      `json:"license_id,omitempty"`
	LicenseKey   string      `json:"license_key,omitempty"` // masked
	CustomerID   string      `json:"customer_id,omitempty"`
	CustomerName string      `json:"customer_name"`
	ResellerID   string      `json:"reseller_id,omitempty"`   // sales channel; empty = direct
	ResellerName string      `json:"reseller_name,omitempty"` // human-friendly reseller label
	Legacy       bool        `json:"legacy,omitempty"`        // grouped by a pre-1.8.0 license instead of a cascade report
	TotalNodes   int         `json:"total_nodes"`
	Roots        []*treeNode `json:"roots"`
}

// treeOperator groups one SOC operator's whole estate: the mysoc platform
// nodes it runs, plus every customer reported through the cascade (or, for
// legacy keys, every customer license sold under it).
type treeOperator struct {
	OperatorID    string          `json:"operator_id,omitempty"`
	OperatorName  string          `json:"operator_name"`
	IsActive      bool            `json:"is_active"`
	TotalNodes    int             `json:"total_nodes"`
	PlatformRoots []*treeNode     `json:"platform_roots"` // the operator's own mysoc instances
	Customers     []*treeCustomer `json:"customers"`
}

type instanceTreeResponse struct {
	Operators []*treeOperator `json:"operators"`
}

// handleInstanceTree assembles the fleet into the sales hierarchy:
// operator -> customer -> siemcore -> swf, with the operator's own mysoc
// platform nodes at the operator level. Parent links resolve across licenses
// (a customer's siemcore legitimately points at the operator's mysoc), so
// orphan now means "parent not enrolled anywhere". Instances with no bound
// license land in an "Unlicensed / unbound" bucket under the Unassigned
// operator.
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
	operatorEntities, err := licenseSvc.ListOperators(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list operators")
		return
	}
	entities := make([]types.Operator, 0, len(operatorEntities))
	for _, op := range operatorEntities {
		entities = append(entities, op.Operator)
	}

	writeJSON(w, http.StatusOK, instanceTreeResponse{
		Operators: groupOperators(instances, licenses, entities),
	})
}

// isPlatformLicense reports whether l is a SOC operator's own mysoc platform
// license, as opposed to a customer's siemcore+swf license.
func isPlatformLicense(l types.License) bool {
	return l.Type == "mysoc-cloud" || (l.OperatorID != "" && l.OperatorID == l.CustomerID)
}

// operatorKeyFor returns the operator bucket a license belongs to ("" when
// unassigned). A platform license without an explicit operator_id (legacy
// row) falls back to its own customer_id.
func operatorKeyFor(l types.License) string {
	if l.OperatorID != "" {
		return l.OperatorID
	}
	if isPlatformLicense(l) {
		return l.CustomerID
	}
	return ""
}

// groupOperators builds the operator -> customer -> instance-tree structure.
//
// New-style (cascade) grouping: an instance whose license carries operator_ref
// belongs to that operator; within the operator, nodes are bucketed by the
// customer_id reported up the cascade. Nodes with no customer (the operator's
// own mysoc platform, typically) become platform roots.
//
// Legacy grouping: instances on pre-1.8.0 licenses (no product column) keep
// the license-based customer buckets, marked Legacy for the dashboard.
func groupOperators(instances []types.Instance, licenses []types.License, entities []types.Operator) []*treeOperator {
	licenseByID := make(map[string]types.License, len(licenses))
	for _, l := range licenses {
		licenseByID[l.ID] = l
	}

	// Every enrolled instance_id, for cross-license parent resolution.
	known := make(map[string]struct{}, len(instances))
	for _, inst := range instances {
		known[inst.InstanceID] = struct{}{}
	}

	operators := map[string]*treeOperator{}
	order := []string{}
	getOp := func(key string) *treeOperator {
		if op, ok := operators[key]; ok {
			return op
		}
		op := &treeOperator{
			OperatorID:    key,
			OperatorName:  key,
			IsActive:      true,
			PlatformRoots: []*treeNode{},
			Customers:     []*treeCustomer{},
		}
		if key == "" {
			op.OperatorName = "Unassigned"
		}
		operators[key] = op
		order = append(order, key)
		return op
	}

	// Operator entities define the canonical buckets and display names.
	for _, e := range entities {
		op := getOp(e.ID)
		op.OperatorName = e.Name
		op.IsActive = e.IsActive
	}
	// Legacy platform licenses may name operators that predate the entity table.
	for _, l := range licenses {
		if !isPlatformLicense(l) {
			continue
		}
		op := getOp(operatorKeyFor(l))
		if op.OperatorName == op.OperatorID && l.CustomerName != "" {
			op.OperatorName = l.CustomerName
		}
	}

	// Split instances: cascade-grouped vs legacy-license vs unlicensed.
	cascade := map[string][]types.Instance{} // operator key -> instances
	legacy := map[string][]types.Instance{}  // license id -> instances
	var unlicensed []types.Instance
	for _, inst := range instances {
		l, ok := licenseByID[inst.LicenseID]
		if !ok {
			unlicensed = append(unlicensed, inst)
			continue
		}
		if l.OperatorRef != "" && l.Product != "" {
			cascade[l.OperatorRef] = append(cascade[l.OperatorRef], inst)
		} else {
			legacy[l.ID] = append(legacy[l.ID], inst)
		}
	}

	// Cascade instances: bucket by reported customer inside each operator.
	for opKey, group := range cascade {
		op := getOp(opKey)
		op.TotalNodes += len(group)

		var platform []types.Instance
		byCustomer := map[string][]types.Instance{}
		customerName := map[string]string{}
		for _, inst := range group {
			if inst.CustomerID == "" {
				platform = append(platform, inst)
				continue
			}
			byCustomer[inst.CustomerID] = append(byCustomer[inst.CustomerID], inst)
			if inst.CustomerName != "" {
				customerName[inst.CustomerID] = inst.CustomerName
			}
		}
		op.PlatformRoots = append(op.PlatformRoots, assembleTree(platform, known)...)
		for customerID, customerGroup := range byCustomer {
			name := customerName[customerID]
			if name == "" {
				name = customerID
			}
			op.Customers = append(op.Customers, &treeCustomer{
				CustomerID:   customerID,
				CustomerName: name,
				TotalNodes:   len(customerGroup),
				Roots:        assembleTree(customerGroup, known),
			})
		}
	}

	// Legacy licenses: platform licenses feed platform roots, customer
	// licenses stay license-shaped buckets (marked Legacy).
	for _, l := range licenses {
		group := legacy[l.ID]
		if isPlatformLicense(l) {
			if len(group) == 0 {
				continue
			}
			op := getOp(operatorKeyFor(l))
			op.PlatformRoots = append(op.PlatformRoots, assembleTree(group, known)...)
			op.TotalNodes += len(group)
			continue
		}
		if len(group) == 0 && l.Product != "" {
			// New-style non-platform licenses do not exist yet; skip quietly.
			continue
		}
		if len(group) == 0 && l.OperatorRef == "" && l.OperatorID == "" {
			// Empty legacy license with no operator affiliation: still show it
			// under Unassigned so a freshly created license is visible.
			op := getOp("")
			op.Customers = append(op.Customers, legacyCustomer(l, nil, known))
			continue
		}
		opKey := l.OperatorRef
		if opKey == "" {
			opKey = operatorKeyFor(l)
		}
		op := getOp(opKey)
		op.Customers = append(op.Customers, legacyCustomer(l, group, known))
		op.TotalNodes += len(group)
	}

	// Instances bound to no (known) license.
	if len(unlicensed) > 0 {
		op := getOp("")
		op.Customers = append(op.Customers, &treeCustomer{
			CustomerName: "Unlicensed / unbound",
			TotalNodes:   len(unlicensed),
			Roots:        assembleTree(unlicensed, known),
		})
		op.TotalNodes += len(unlicensed)
	}

	out := make([]*treeOperator, 0, len(order))
	for _, key := range order {
		op := operators[key]
		sort.SliceStable(op.Customers, func(i, j int) bool {
			ci, cj := op.Customers[i], op.Customers[j]
			if (ci.CustomerID == "") != (cj.CustomerID == "") {
				return ci.CustomerID != "" // real customers before the unlicensed bucket
			}
			return strings.ToLower(ci.CustomerName) < strings.ToLower(cj.CustomerName)
		})
		out = append(out, op)
	}

	// Named operators first (by name); the Unassigned bucket last.
	sort.SliceStable(out, func(i, j int) bool {
		oi, oj := out[i], out[j]
		if (oi.OperatorID == "") != (oj.OperatorID == "") {
			return oi.OperatorID != ""
		}
		return strings.ToLower(oi.OperatorName) < strings.ToLower(oj.OperatorName)
	})
	return out
}

// legacyCustomer builds the license-shaped customer bucket used for
// pre-1.8.0 licenses.
func legacyCustomer(l types.License, group []types.Instance, known map[string]struct{}) *treeCustomer {
	cust := &treeCustomer{
		LicenseID:    l.ID,
		LicenseKey:   maskLicenseKey(l.LicenseKey),
		CustomerID:   l.CustomerID,
		CustomerName: l.CustomerName,
		ResellerID:   l.ResellerID,
		ResellerName: l.ResellerName,
		Legacy:       true,
		TotalNodes:   len(group),
		Roots:        assembleTree(group, known),
	}
	if cust.CustomerName == "" {
		cust.CustomerName = "Unnamed customer"
	}
	return cust
}

// assembleTree links one license bucket's instances into a tier tree. A node
// nests under its parent when the parent shares the license. A node whose
// parent lives elsewhere in the fleet (e.g. a customer siemcore pointing at
// the operator's mysoc) stays a root of this bucket without being flagged;
// Orphan is set only when the declared parent is not enrolled anywhere.
func assembleTree(group []types.Instance, known map[string]struct{}) []*treeNode {
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
			CustomerID:       inst.CustomerID,
			Version:          primaryVersion(inst.LastHeartbeatData),
			Status:           inst.Status,
			LastHeartbeat:    inst.LastHeartbeat,
			ReportedVia:      inst.ReportedVia,
			ReportedAt:       inst.ReportedAt,
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
		if _, enrolled := known[n.ParentInstanceID]; !enrolled {
			// Declared a parent that is not enrolled anywhere in the fleet.
			n.Orphan = true
		}
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

// primaryVersion extracts the first product's version from a heartbeat snapshot.
func primaryVersion(hb *types.Heartbeat) string {
	if hb == nil || len(hb.Products) == 0 {
		return ""
	}
	return hb.Products[0].Version
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
