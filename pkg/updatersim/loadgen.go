package updatersim

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// LoadOptions configures the Fleet Scalability 1.12 load rig: it synthesizes N
// customer subtrees (one siemcore relay + M swf leaves each) and drives them at
// a target as delta-reporting heartbeats, so the server's delta ingest path is
// exercised at 20k customers / 100k+ nodes without standing up real relays.
type LoadOptions struct {
	// TargetURL is the base URL of the updates server (or a mysoc relay). The
	// rig posts to TargetURL + "/api/v1/heartbeat".
	TargetURL string
	// LicenseKey is presented as X-License-Key on every heartbeat.
	LicenseKey string
	// Customers is the number of synthetic customer subtrees.
	Customers int
	// LeavesPerCustomer is the swf-leaf count under each customer relay.
	LeavesPerCustomer int
	// Cycles is how many heartbeat rounds to drive. Cycle 1 enrolls the whole
	// fleet (full inventory per customer); later cycles carry only the churn.
	Cycles int
	// Concurrency bounds simultaneous in-flight heartbeats.
	Concurrency int
	// ChurnPercent is the fraction of each customer's leaves that change in a
	// steady (post-enrollment) cycle, driving the delta stream size.
	ChurnPercent int
	// ParentInstanceID is the mysoc relay the synthetic customers report to.
	ParentInstanceID string
	// Insecure skips TLS verification (self-signed relay certs).
	Insecure bool
	// Timeout bounds a single heartbeat request.
	Timeout time.Duration
}

// CycleReport captures one cycle's acceptance-gate metrics.
type CycleReport struct {
	Cycle      int
	Requests   int
	Errors     int
	Nodes      int           // inventory rows carried this cycle
	Elapsed    time.Duration // wall time for the whole cycle
	Throughput float64       // requests/second
	P50, P95   time.Duration
	P99, Max   time.Duration
}

// String renders a report as a single acceptance-gate line.
func (r CycleReport) String() string {
	return fmt.Sprintf(
		"cycle %d: %d req, %d err, %d nodes, %.0f req/s, p50=%s p95=%s p99=%s max=%s (wall %s)",
		r.Cycle, r.Requests, r.Errors, r.Nodes, r.Throughput,
		r.P50.Round(time.Millisecond), r.P95.Round(time.Millisecond),
		r.P99.Round(time.Millisecond), r.Max.Round(time.Millisecond),
		r.Elapsed.Round(time.Millisecond),
	)
}

// RunLoad drives the whole fleet for the configured number of cycles, invoking
// report after each cycle. It returns the last error encountered (requests
// keep flowing past individual failures so a run yields full metrics).
func RunLoad(ctx context.Context, o LoadOptions, report func(CycleReport)) error {
	o.applyDefaults()
	client := &http.Client{
		Timeout: o.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        o.Concurrency * 2,
			MaxIdleConnsPerHost: o.Concurrency * 2,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: o.Insecure}, //nolint:gosec // load rig only
		},
	}
	url := o.TargetURL + "/api/v1/heartbeat"

	var lastErr error
	for cycle := 1; cycle <= o.Cycles && ctx.Err() == nil; cycle++ {
		rep, err := o.runCycle(ctx, client, url, cycle)
		if err != nil {
			lastErr = err
		}
		if report != nil {
			report(rep)
		}
	}
	return lastErr
}

func (o *LoadOptions) applyDefaults() {
	if o.Customers <= 0 {
		o.Customers = 20000
	}
	if o.LeavesPerCustomer < 0 {
		o.LeavesPerCustomer = 0
	}
	if o.Cycles <= 0 {
		o.Cycles = 2
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 64
	}
	if o.ChurnPercent <= 0 {
		o.ChurnPercent = 2
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
}

// runCycle dispatches one heartbeat per customer across the worker pool and
// aggregates latency percentiles.
func (o *LoadOptions) runCycle(ctx context.Context, client *http.Client, url string, cycle int) (CycleReport, error) {
	latencies := make([]time.Duration, o.Customers)
	var errCount, nodeCount int64
	var firstErr atomic.Pointer[error]

	jobs := make(chan int, o.Concurrency)
	var wg sync.WaitGroup
	start := time.Now()

	for w := 0; w < o.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				hb := o.buildCustomerHeartbeat(cycle, i)
				if hb.Delta != nil {
					atomic.AddInt64(&nodeCount, int64(len(hb.Delta.Inventory)))
				}
				lat, err := postHeartbeat(ctx, client, url, o.LicenseKey, hb)
				latencies[i] = lat
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					e := err
					firstErr.CompareAndSwap(nil, &e)
				}
			}
		}()
	}
	for i := 0; i < o.Customers && ctx.Err() == nil; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	elapsed := time.Since(start)
	rep := CycleReport{
		Cycle:    cycle,
		Requests: o.Customers,
		Errors:   int(errCount),
		Nodes:    int(nodeCount),
		Elapsed:  elapsed,
	}
	if elapsed > 0 {
		rep.Throughput = float64(o.Customers) / elapsed.Seconds()
	}
	rep.P50, rep.P95, rep.P99, rep.Max = percentiles(latencies)

	var err error
	if p := firstErr.Load(); p != nil {
		err = *p
	}
	return rep, err
}

// buildCustomerHeartbeat synthesizes one customer relay's delta heartbeat. On
// the enrollment cycle it carries every leaf; on steady cycles it carries only
// the churned subset plus the refreshed summary — exactly the O(changes) shape
// a real delta-reporting relay produces.
func (o *LoadOptions) buildCustomerHeartbeat(cycle, idx int) platformtypes.Heartbeat {
	customerID := fmt.Sprintf("loadgen-cust-%06d", idx)
	relayID := fmt.Sprintf("loadgen-siemcore-%06d", idx)

	changing := o.LeavesPerCustomer
	if cycle > 1 {
		changing = o.LeavesPerCustomer * o.ChurnPercent / 100
		if changing == 0 && o.LeavesPerCustomer > 0 {
			changing = 1
		}
	}

	var seq uint64
	env := &platformtypes.DeltaEnvelope{}
	version := fmt.Sprintf("1.%d.0", cycle) // version advances each cycle => a real change
	for l := 0; l < changing; l++ {
		seq++
		env.Inventory = append(env.Inventory, platformtypes.InventoryChange{
			Seq: seq,
			Node: platformtypes.ChildReport{
				InstanceID:       fmt.Sprintf("%s-leaf-%04d", relayID, l),
				ProductTier:      TierSWF,
				ParentInstanceID: relayID,
				CustomerID:       customerID,
				Status:           "online",
				LastSeen:         time.Now().UTC(),
				Products:         []platformtypes.ProductStatus{{Name: "swf", Version: version}},
			},
		})
	}
	seq++
	env.Summaries = append(env.Summaries, platformtypes.FleetSummary{
		CustomerID:       customerID,
		CustomerName:     fmt.Sprintf("Loadgen Customer %06d", idx),
		ReporterID:       relayID,
		Total:            o.LeavesPerCustomer,
		Online:           o.LeavesPerCustomer,
		FailedUpdates:    0,
		Versions:         map[string]int{"swf@" + version: changing},
		StatusReportedAt: time.Now().UTC(),
	})
	env.Cursor = seq

	return platformtypes.Heartbeat{
		InstanceID:       relayID,
		InstanceType:     "server",
		ProductTier:      TierSiemCore,
		ParentInstanceID: o.ParentInstanceID,
		CustomerID:       customerID,
		CustomerName:     fmt.Sprintf("Loadgen Customer %06d", idx),
		Hostname:         relayID,
		UpdaterVersion:   "loadgen/1.12",
		Products:         []platformtypes.ProductStatus{{Name: "siemcore", Version: version}},
		Timestamp:        time.Now().UTC(),
		Delta:            env,
	}
}

func postHeartbeat(ctx context.Context, client *http.Client, url, licenseKey string, hb platformtypes.Heartbeat) (time.Duration, error) {
	body, err := json.Marshal(hb)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if licenseKey != "" {
		req.Header.Set("X-License-Key", licenseKey)
	}
	start := time.Now()
	resp, err := client.Do(req)
	lat := time.Since(start)
	if err != nil {
		return lat, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return lat, fmt.Errorf("heartbeat %s: status %d", hb.InstanceID, resp.StatusCode)
	}
	return lat, nil
}

// percentiles returns p50/p95/p99/max from a latency sample. A copy is sorted
// so the caller's slice order is preserved.
func percentiles(samples []time.Duration) (p50, p95, p99, max time.Duration) {
	if len(samples) == 0 {
		return 0, 0, 0, 0
	}
	s := make([]time.Duration, len(samples))
	copy(s, samples)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	pick := func(q float64) time.Duration {
		idx := int(q * float64(len(s)-1))
		return s[idx]
	}
	return pick(0.50), pick(0.95), pick(0.99), s[len(s)-1]
}
