package api

import (
	"testing"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func inst(instanceID, tier, parent string) types.Instance {
	return types.Instance{
		ID:               "id-" + instanceID,
		InstanceID:       instanceID,
		ProductTier:      tier,
		ParentInstanceID: parent,
		Status:           "online",
	}
}

// knownSet builds the fleet-wide instance_id set from one or more buckets.
func knownSet(groups ...[]types.Instance) map[string]struct{} {
	known := map[string]struct{}{}
	for _, group := range groups {
		for _, i := range group {
			known[i.InstanceID] = struct{}{}
		}
	}
	return known
}

func TestAssembleTreeNestsByTier(t *testing.T) {
	group := []types.Instance{
		inst("swf-1", "swf", "siem-1"),
		inst("mysoc-1", "mysoc", ""),
		inst("siem-1", "siemcore", "mysoc-1"),
	}

	roots := assembleTree(group, knownSet(group))
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	root := roots[0]
	if root.InstanceID != "mysoc-1" || root.ProductTier != "mysoc" {
		t.Fatalf("root = %+v, want mysoc-1/mysoc", root)
	}
	if len(root.Children) != 1 || root.Children[0].InstanceID != "siem-1" {
		t.Fatalf("expected siem-1 under mysoc-1, got %+v", root.Children)
	}
	siem := root.Children[0]
	if len(siem.Children) != 1 || siem.Children[0].InstanceID != "swf-1" {
		t.Fatalf("expected swf-1 under siem-1, got %+v", siem.Children)
	}
	if siem.Children[0].Orphan {
		t.Fatal("swf-1 has a resolvable parent and must not be marked orphan")
	}
}

func TestAssembleTreeFlagsOrphans(t *testing.T) {
	group := []types.Instance{
		inst("swf-orphan", "swf", "siem-missing"),
		inst("mysoc-1", "mysoc", ""),
	}
	roots := assembleTree(group, knownSet(group))
	// mysoc-1 (rank 0) sorts before the orphan swf (rank 2).
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots (mysoc + orphan), got %d", len(roots))
	}
	if roots[0].InstanceID != "mysoc-1" {
		t.Fatalf("expected mysoc-1 first by rank, got %q", roots[0].InstanceID)
	}
	orphan := roots[1]
	if orphan.InstanceID != "swf-orphan" || !orphan.Orphan {
		t.Fatalf("expected swf-orphan flagged as orphan, got %+v", orphan)
	}
}

func TestAssembleTreeCrossLicenseParentIsNotOrphan(t *testing.T) {
	// The operator's mysoc lives in another license bucket; the customer's
	// siemcore pointing at it is a legitimate root, not an orphan.
	operatorBucket := []types.Instance{inst("mysoc-op", "mysoc", "")}
	customerBucket := []types.Instance{
		inst("siem-1", "siemcore", "mysoc-op"),
		inst("swf-1", "swf", "siem-1"),
	}

	roots := assembleTree(customerBucket, knownSet(operatorBucket, customerBucket))
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].InstanceID != "siem-1" || roots[0].Orphan {
		t.Fatalf("siem-1 must be an unflagged root, got %+v", roots[0])
	}
	if len(roots[0].Children) != 1 || roots[0].Children[0].InstanceID != "swf-1" {
		t.Fatalf("expected swf-1 nested under siem-1, got %+v", roots[0].Children)
	}
}

func TestAssembleTreeSortsSiblingsByRankThenID(t *testing.T) {
	group := []types.Instance{
		inst("mysoc-1", "mysoc", ""),
		inst("swf-b", "swf", "mysoc-1"), // orphan-ish: wrong parent tier, but parent exists so it nests
		inst("siem-2", "siemcore", "mysoc-1"),
		inst("siem-1", "siemcore", "mysoc-1"),
	}
	roots := assembleTree(group, knownSet(group))
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	children := roots[0].Children
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}
	// siemcore (rank 1) before swf (rank 2); within siemcore, by instance_id.
	wantOrder := []string{"siem-1", "siem-2", "swf-b"}
	for i, want := range wantOrder {
		if children[i].InstanceID != want {
			t.Fatalf("children[%d] = %q, want %q", i, children[i].InstanceID, want)
		}
	}
}

func lic(id, customerID, name, licType, operatorID, resellerID string) types.License {
	return types.License{
		ID:           id,
		LicenseKey:   "SIEM-AAAA-BBBB-CCCC-DDDD",
		CustomerID:   customerID,
		CustomerName: name,
		Type:         licType,
		OperatorID:   operatorID,
		ResellerID:   resellerID,
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		IsActive:     true,
	}
}

func withLicense(i types.Instance, licenseID string) types.Instance {
	i.LicenseID = licenseID
	return i
}

func TestGroupOperatorsSalesHierarchy(t *testing.T) {
	licenses := []types.License{
		lic("op-lic", "cyfox-soc", "Cyfox SOC", "mysoc-cloud", "cyfox-soc", ""),
		lic("cust-a", "acme", "Acme Corp", "siemcore", "cyfox-soc", ""),
		lic("cust-b", "beta", "Beta Ltd", "siemcore", "cyfox-soc", "chan-1"),
		lic("cust-x", "nobody", "No Operator Yet", "siemcore", "", ""),
	}
	instances := []types.Instance{
		withLicense(inst("mysoc-op", "mysoc", ""), "op-lic"),
		withLicense(inst("siem-a1", "siemcore", "mysoc-op"), "cust-a"),
		withLicense(inst("swf-a1", "swf", "siem-a1"), "cust-a"),
		inst("stray", "swf", ""), // no license at all
	}

	ops := groupOperators(instances, licenses, nil)
	if len(ops) != 2 {
		t.Fatalf("expected 2 operators (cyfox + unassigned), got %d", len(ops))
	}

	cyfox := ops[0]
	if cyfox.OperatorID != "cyfox-soc" || cyfox.OperatorName != "Cyfox SOC" {
		t.Fatalf("first operator = %s/%s, want cyfox-soc/Cyfox SOC", cyfox.OperatorID, cyfox.OperatorName)
	}
	if len(cyfox.PlatformRoots) != 1 || cyfox.PlatformRoots[0].InstanceID != "mysoc-op" {
		t.Fatalf("expected mysoc-op as platform root, got %+v", cyfox.PlatformRoots)
	}
	if cyfox.TotalNodes != 3 {
		t.Fatalf("cyfox total nodes = %d, want 3", cyfox.TotalNodes)
	}
	// Customers sorted by name: Acme (with instances) then Beta (empty license).
	if len(cyfox.Customers) != 2 {
		t.Fatalf("expected 2 customers under cyfox, got %d", len(cyfox.Customers))
	}
	acme := cyfox.Customers[0]
	if acme.CustomerID != "acme" || len(acme.Roots) != 1 || acme.Roots[0].InstanceID != "siem-a1" {
		t.Fatalf("acme customer wrong: %+v", acme)
	}
	if acme.Roots[0].Orphan {
		t.Fatal("siem-a1 links cross-license to mysoc-op and must not be orphan")
	}
	if len(acme.Roots[0].Children) != 1 || acme.Roots[0].Children[0].InstanceID != "swf-a1" {
		t.Fatalf("expected swf-a1 under siem-a1, got %+v", acme.Roots[0].Children)
	}
	beta := cyfox.Customers[1]
	if beta.CustomerID != "beta" || beta.ResellerID != "chan-1" || len(beta.Roots) != 0 {
		t.Fatalf("beta customer wrong (must appear with empty tree + reseller): %+v", beta)
	}

	unassigned := ops[1]
	if unassigned.OperatorID != "" || unassigned.OperatorName != "Unassigned" {
		t.Fatalf("last operator must be Unassigned, got %s/%s", unassigned.OperatorID, unassigned.OperatorName)
	}
	// One customer license without operator + the unlicensed bucket (last).
	if len(unassigned.Customers) != 2 {
		t.Fatalf("expected 2 buckets under Unassigned, got %d", len(unassigned.Customers))
	}
	if unassigned.Customers[0].CustomerID != "nobody" {
		t.Fatalf("expected operator-less license first, got %+v", unassigned.Customers[0])
	}
	unlicensed := unassigned.Customers[1]
	if unlicensed.LicenseID != "" || unlicensed.CustomerName != "Unlicensed / unbound" {
		t.Fatalf("expected unlicensed bucket last, got %+v", unlicensed)
	}
	if len(unlicensed.Roots) != 1 || unlicensed.Roots[0].InstanceID != "stray" {
		t.Fatalf("expected stray in unlicensed bucket, got %+v", unlicensed.Roots)
	}
}

func TestGroupOperatorsCascadeModel(t *testing.T) {
	// 1.8.0 model: one operator entity, one platform key (product=mysoc,
	// operator_ref set); the fleet arrives via the cascade with customer_id
	// stamped on each node.
	platform := lic("op-key", "cyfox-soc", "Cyfox SOC", "mysoc-cloud", "", "")
	platform.Product = "mysoc"
	platform.OperatorRef = "cyfox-soc"
	licenses := []types.License{platform}
	entities := []types.Operator{{ID: "cyfox-soc", Name: "Cyfox SOC Ltd", IsActive: true}}

	withCustomer := func(i types.Instance, customerID, customerName, reportedVia string) types.Instance {
		i.CustomerID = customerID
		i.CustomerName = customerName
		i.ReportedVia = reportedVia
		return i
	}
	instances := []types.Instance{
		withLicense(inst("mysoc-op", "mysoc", ""), "op-key"),
		withLicense(withCustomer(inst("siem-a1", "siemcore", "mysoc-op"), "acme", "Acme Corp", "mysoc-op"), "op-key"),
		withLicense(withCustomer(inst("swf-a1", "swf", "siem-a1"), "acme", "Acme Corp", "mysoc-op"), "op-key"),
		withLicense(withCustomer(inst("siem-b1", "siemcore", "mysoc-op"), "beta", "", "mysoc-op"), "op-key"),
	}

	ops := groupOperators(instances, licenses, entities)
	if len(ops) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(ops))
	}
	op := ops[0]
	if op.OperatorID != "cyfox-soc" || op.OperatorName != "Cyfox SOC Ltd" {
		t.Fatalf("operator = %s/%s, want cyfox-soc/Cyfox SOC Ltd (entity name wins)", op.OperatorID, op.OperatorName)
	}
	if op.TotalNodes != 4 {
		t.Fatalf("total nodes = %d, want 4", op.TotalNodes)
	}
	if len(op.PlatformRoots) != 1 || op.PlatformRoots[0].InstanceID != "mysoc-op" {
		t.Fatalf("expected mysoc-op as the platform root, got %+v", op.PlatformRoots)
	}
	if len(op.Customers) != 2 {
		t.Fatalf("expected 2 reported customers, got %d", len(op.Customers))
	}
	acme := op.Customers[0]
	if acme.CustomerID != "acme" || acme.CustomerName != "Acme Corp" || acme.Legacy {
		t.Fatalf("acme bucket wrong: %+v", acme)
	}
	if len(acme.Roots) != 1 || acme.Roots[0].InstanceID != "siem-a1" || acme.Roots[0].Orphan {
		t.Fatalf("acme roots wrong: %+v", acme.Roots)
	}
	if acme.Roots[0].ReportedVia != "mysoc-op" {
		t.Fatalf("expected siem-a1 reported via mysoc-op, got %q", acme.Roots[0].ReportedVia)
	}
	if len(acme.Roots[0].Children) != 1 || acme.Roots[0].Children[0].InstanceID != "swf-a1" {
		t.Fatalf("expected swf-a1 nested under siem-a1, got %+v", acme.Roots[0].Children)
	}
	beta := op.Customers[1]
	if beta.CustomerID != "beta" || beta.CustomerName != "beta" {
		t.Fatalf("beta bucket must fall back to customer_id as name, got %+v", beta)
	}
}

func TestMaskLicenseKey(t *testing.T) {
	cases := map[string]string{
		"SIEM-FE94-A129-BF44-A9C9": "SIEM…A9C9",
		"short":                    "****",
		"":                         "",
		"  SIEM-FE94-A129  ":       "SIEM…A129",
	}
	for in, want := range cases {
		if got := maskLicenseKey(in); got != want {
			t.Fatalf("maskLicenseKey(%q) = %q, want %q", in, got, want)
		}
	}
}
