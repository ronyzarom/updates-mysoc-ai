package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// Relay serves the updater API subset to child updaters while remaining a
// normal updater toward its own parent. It is the only distribution component
// in the cascade: no siemcore/swf/mysoc product code is involved.
//
//   - POST /api/v1/heartbeat           — child check-in; stored for the rollup
//   - POST /api/v1/decommission        — child announces its own clean removal
//   - POST /api/v1/updates/{p}/check   — forwarded upstream with the relay's credential
//   - POST /api/v1/updates/{p}/report  — recorded and carried up in the rollup
//   - GET  /api/v1/releases/{p}/{v}/download — pull-through verified cache
//   - GET  /health                     — liveness + child count
type Relay struct {
	config    *Config
	upstream  *Client
	logger    *slog.Logger
	publicKey ed25519.PublicKey // nil = signature verification disabled
	guard     *relayGuard       // port self-protection (see relayguard.go)

	mu       sync.Mutex
	children map[string]*childState

	cacheMu sync.Mutex // serializes pull-through fetches
}

type childState struct {
	Heartbeat  platformtypes.Heartbeat
	LastSeen   time.Time
	Token      string
	LastReport *platformtypes.UpdateAttempt
	SourceIP   string // address the child last connected from
	// Decommissioned marks a child that announced its own removal
	// (POST /api/v1/decommission). The mark rolls up as a child status and is
	// cleared by any subsequent genuine heartbeat (honest revival).
	Decommissioned   bool
	DecommissionedAt time.Time
}

// decommissionRetention bounds how long a decommissioned child stays in relay
// state after the mark: the updates server row is the durable record, the
// relay only needs to deliver the status via rollups for a while.
const decommissionRetention = 7 * 24 * time.Hour

// NewRelay builds the relay listener component.
func NewRelay(cfg *Config, upstream *Client, logger *slog.Logger) (*Relay, error) {
	if !cfg.Relay.Enabled {
		return nil, fmt.Errorf("relay is not enabled in configuration")
	}
	if logger == nil {
		logger = slog.Default()
	}
	relay := &Relay{
		config:   cfg,
		upstream: upstream,
		logger:   logger,
		guard:    newRelayGuard(),
		children: map[string]*childState{},
	}
	if pubHex := strings.TrimSpace(cfg.Signing.PublicKey); pubHex != "" {
		pub, err := signing.ParsePublicKeyHex(pubHex)
		if err != nil {
			return nil, fmt.Errorf("relay signing key: %w", err)
		}
		relay.publicKey = pub
	}
	if err := os.MkdirAll(cfg.Relay.CacheDir, 0700); err != nil {
		return nil, fmt.Errorf("create relay cache dir: %w", err)
	}
	relay.loadTombstones()
	return relay, nil
}

// Serve runs the relay listener until the context is canceled.
func (r *Relay) Serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", r.handleHealth)
	mux.HandleFunc("POST /api/v1/heartbeat", r.handleChildHeartbeat)
	mux.HandleFunc("POST /api/v1/decommission", r.handleChildDecommission)
	mux.HandleFunc("POST /api/v1/updates/{product}/check", r.handleChildCheck)
	mux.HandleFunc("POST /api/v1/updates/{product}/report", r.handleChildReport)
	mux.HandleFunc("GET /api/v1/releases/{product}/{version}", r.handleChildReleaseMeta)
	mux.HandleFunc("GET /api/v1/releases/{product}/{version}/download", r.handleChildDownload)

	// The child-facing listener is TLS-only: the child token and telemetry
	// must never cross the network in cleartext.
	certFile, keyFile, err := ensureRelayTLS(r.config, r.logger)
	if err != nil {
		return fmt.Errorf("relay TLS material: %w", err)
	}

	// The guard fronts every route: temp-bans, per-IP rate limits, and the
	// learned-source restriction are applied before any handler runs.
	server := &http.Server{
		Addr:              r.config.Relay.Listen,
		Handler:           r.guard.middleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    16 << 10,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	errCh := make(chan error, 1)
	go func() {
		r.logger.Info("relay listening",
			"listen", r.config.Relay.Listen,
			"tls_cert", certFile,
			"cache_dir", r.config.Relay.CacheDir,
			"signature_verification", r.publicKey != nil,
		)
		errCh <- server.ListenAndServeTLS(certFile, keyFile)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// ChildrenReport builds the recursive fleet rollup for the relay's own upward
// heartbeat. Children that are themselves relays contribute their subtree via
// the Children field of their stored heartbeat.
func (r *Relay) ChildrenReport() []platformtypes.ChildReport {
	r.mu.Lock()
	defer r.mu.Unlock()

	offlineAfter := r.config.Relay.ChildOfflineAfter.Duration
	now := time.Now()
	pruned := false
	reports := make([]platformtypes.ChildReport, 0, len(r.children))
	for id, child := range r.children {
		// Decommissioned entries are kept only long enough to deliver the
		// status upward; the server row is the durable record.
		if child.Decommissioned && now.Sub(child.DecommissionedAt) > decommissionRetention {
			delete(r.children, id)
			pruned = true
			continue
		}
		hb := child.Heartbeat
		status := "online"
		lastSeen := child.LastSeen
		if offlineAfter > 0 && now.Sub(lastSeen) > offlineAfter {
			status = "offline"
		}
		if child.Decommissioned {
			status = "decommissioned"
			// The mark postdates any heartbeat or forwarded check by this
			// child; reporting it as the last-seen moment keeps the server's
			// rollup freshness guard from discarding the status.
			lastSeen = child.DecommissionedAt
		}
		attempt := hb.LastUpdateAttempt
		if attempt == nil {
			attempt = child.LastReport
		}
		system := hb.System
		reports = append(reports, platformtypes.ChildReport{
			InstanceID:        hb.InstanceID,
			InstanceType:      hb.InstanceType,
			ProductTier:       hb.ProductTier,
			CustomerID:        hb.CustomerID,
			CustomerName:      hb.CustomerName,
			Hostname:          hb.Hostname,
			UpdaterVersion:    hb.UpdaterVersion,
			Products:          hb.Products,
			System:            &system,
			Status:            status,
			LastSeen:          lastSeen,
			LastUpdateAttempt: attempt,
			Children:          hb.Children,
			SourceIP:          child.SourceIP,
		})
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].InstanceID < reports[j].InstanceID })
	if pruned {
		r.saveTombstonesLocked()
	}
	return reports
}

// ---- child-facing handlers ----

func (r *Relay) handleHealth(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	count := len(r.children)
	r.mu.Unlock()
	relayJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"role":     "relay",
		"children": count,
	})
}

// handleChildHeartbeat enrolls or refreshes a child. The first contact issues
// a relay token; later contacts must present it (trust on first contact — a
// relay restart re-adopts the token the child presents).
func (r *Relay) handleChildHeartbeat(w http.ResponseWriter, req *http.Request) {
	var heartbeat platformtypes.Heartbeat
	if err := json.NewDecoder(io.LimitReader(req.Body, 4<<20)).Decode(&heartbeat); err != nil {
		relayError(w, http.StatusBadRequest, "invalid heartbeat body")
		return
	}
	if strings.TrimSpace(heartbeat.InstanceID) == "" {
		relayError(w, http.StatusBadRequest, "instance_id is required")
		return
	}
	if strings.TrimSpace(req.Header.Get("X-License-Key")) == "" {
		r.guard.noteAuthFailure(req.RemoteAddr)
		relayError(w, http.StatusUnauthorized, "a child credential (X-License-Key) is required")
		return
	}

	presented := strings.TrimSpace(req.Header.Get("X-Relay-Token"))

	r.mu.Lock()
	child, known := r.children[heartbeat.InstanceID]
	issuedToken := ""
	switch {
	case !known:
		// Bound the child map: an internet-reachable listener must not let
		// arbitrary instance_ids balloon relay memory.
		if len(r.children) >= guardMaxChildren {
			r.mu.Unlock()
			r.logger.Warn("relay children capacity reached; rejecting new enrollment",
				"instance_id", heartbeat.InstanceID, "remote", req.RemoteAddr)
			relayError(w, http.StatusTooManyRequests, "relay children capacity reached")
			return
		}
		token := presented
		if token == "" {
			token = newRelayToken()
			issuedToken = token
		}
		child = &childState{Token: token}
		r.children[heartbeat.InstanceID] = child
	case child.Token != "" && presented != child.Token:
		r.mu.Unlock()
		r.guard.noteAuthFailure(req.RemoteAddr)
		relayError(w, http.StatusUnauthorized, "relay token mismatch for this instance_id")
		return
	}
	child.Heartbeat = heartbeat
	child.LastSeen = time.Now()
	child.SourceIP = remoteIP(req.RemoteAddr)
	// A genuine heartbeat contradicts a decommission mark: revive (honest
	// revival — reinstall without --purge, or a false mark from a leaked
	// credential, which this makes visible).
	revived := child.Decommissioned
	child.Decommissioned = false
	child.DecommissionedAt = time.Time{}
	if revived {
		r.saveTombstonesLocked()
	}
	r.mu.Unlock()
	r.guard.noteAuthSuccess(req.RemoteAddr)

	if issuedToken != "" {
		r.logger.Info("child enrolled at relay",
			"instance_id", heartbeat.InstanceID,
			"tier", heartbeat.ProductTier,
			"customer_id", heartbeat.CustomerID,
		)
	}
	if revived {
		r.logger.Warn("decommissioned child revived by heartbeat",
			"instance_id", heartbeat.InstanceID, "remote", req.RemoteAddr)
	}

	relayJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "ok",
		"updates":     []interface{}{},
		"relay_token": issuedToken,
		// Identity adoption (contract 1.11.0 Item A): the relay attests what
		// it knows so a bootstrap needs only the relay host. Static values;
		// clients adopt once and never mutate on later responses.
		"identity": r.identityObject(),
	})
}

// identityObject builds the relay-attested identity for enrollment responses:
// always the relay's own instance id as the parent, plus the customer
// identity when configured. Unknown fields are omitted, never empty strings.
func (r *Relay) identityObject() map[string]string {
	identity := map[string]string{
		"parent_instance_id": strings.TrimSpace(r.config.Instance.ID),
	}
	if c := strings.TrimSpace(r.config.Instance.CustomerID); c != "" {
		identity["customer_id"] = c
	}
	if n := strings.TrimSpace(r.config.Instance.CustomerName); n != "" {
		identity["customer_name"] = n
	}
	return identity
}

// handleChildDecommission marks a child as cleanly removed (contract 1.11.0
// Item B). Idempotent by design: repeat calls, already-decommissioned and
// unknown instance ids all ack — a goodbye is never worth retrying into a
// ban. The mark reaches the updates server as a child status in the next
// rollup and is cleared by any subsequent genuine heartbeat.
func (r *Relay) handleChildDecommission(w http.ResponseWriter, req *http.Request) {
	var body struct {
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(io.LimitReader(req.Body, 1<<20)).Decode(&body); err != nil {
		relayError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	instanceID := strings.TrimSpace(body.InstanceID)
	if instanceID == "" {
		relayError(w, http.StatusBadRequest, "instance_id is required")
		return
	}
	if !r.authorizeChild(w, req, instanceID) {
		return
	}

	now := time.Now()
	r.mu.Lock()
	child, known := r.children[instanceID]
	if !known {
		// Tombstone: the relay may have restarted and forgotten the child;
		// the mark must still roll up. Bounded like enrollment — on a full
		// relay the call still acks (best-effort goodbye), it just cannot
		// be recorded.
		if len(r.children) >= guardMaxChildren {
			r.mu.Unlock()
			r.logger.Warn("relay at children capacity; decommission mark dropped",
				"instance_id", instanceID)
			relayJSON(w, http.StatusOK, map[string]string{"status": "decommissioned"})
			return
		}
		child = &childState{
			Heartbeat: platformtypes.Heartbeat{InstanceID: instanceID},
			LastSeen:  now,
		}
		r.children[instanceID] = child
	}
	already := child.Decommissioned
	child.Decommissioned = true
	if child.DecommissionedAt.IsZero() {
		child.DecommissionedAt = now
	}
	r.saveTombstonesLocked()
	r.mu.Unlock()

	if !already {
		r.logger.Info("child decommissioned",
			"instance_id", instanceID, "remote", req.RemoteAddr)
	}
	relayJSON(w, http.StatusOK, map[string]string{"status": "decommissioned"})
}

// tombstonePath is the persisted set of decommission marks: a relay restart
// must not lose a mark before the rollup has delivered it.
func (r *Relay) tombstonePath() string {
	return filepath.Join(r.config.Relay.CacheDir, "decommissioned.json")
}

// saveTombstonesLocked persists current decommission marks; r.mu must be held.
func (r *Relay) saveTombstonesLocked() {
	snapshot := map[string]time.Time{}
	for id, child := range r.children {
		if child.Decommissioned {
			snapshot[id] = child.DecommissionedAt
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	if err := os.WriteFile(r.tombstonePath(), data, 0600); err != nil {
		r.logger.Warn("persist decommission tombstones", "error", err)
	}
}

// loadTombstones restores decommission marks at startup, dropping expired ones.
func (r *Relay) loadTombstones() {
	data, err := os.ReadFile(r.tombstonePath())
	if err != nil {
		return
	}
	var snapshot map[string]time.Time
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, at := range snapshot {
		if now.Sub(at) > decommissionRetention || len(r.children) >= guardMaxChildren {
			continue
		}
		child, ok := r.children[id]
		if !ok {
			child = &childState{
				Heartbeat: platformtypes.Heartbeat{InstanceID: id},
				LastSeen:  at,
			}
			r.children[id] = child
		}
		child.Decommissioned = true
		child.DecommissionedAt = at
	}
}

// handleChildCheck forwards the child's update check upstream using the
// relay's own credential, then rewrites the download URL to point at this
// relay so the artifact is served from the verified cache.
func (r *Relay) handleChildCheck(w http.ResponseWriter, req *http.Request) {
	product := req.PathValue("product")
	if !productNamePattern.MatchString(product) {
		relayError(w, http.StatusBadRequest, "invalid product name")
		return
	}
	var checkReq UpdateCheckRequest
	if err := json.NewDecoder(io.LimitReader(req.Body, 1<<20)).Decode(&checkReq); err != nil {
		relayError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !r.authorizeChild(w, req, checkReq.InstanceID) {
		return
	}

	offer, err := r.upstream.CheckUpdate(req.Context(), product, checkReq)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			relayError(w, apiErr.StatusCode, apiErr.Message)
			return
		}
		relayError(w, http.StatusBadGateway, "upstream check failed: "+err.Error())
		return
	}

	if !offer.UpdateAvailable {
		relayJSON(w, http.StatusOK, map[string]interface{}{
			"update_available": false,
			"current_version":  checkReq.CurrentVersion,
			"update_group":     offer.UpdateGroup,
		})
		return
	}

	// The child downloads from this relay, never directly upstream.
	localURL := fmt.Sprintf("/api/v1/releases/%s/%s/download", product, offer.LatestVersion)
	relayJSON(w, http.StatusOK, map[string]interface{}{
		"update_available": true,
		"latest_version":   offer.LatestVersion,
		"download_url":     localURL,
		"update_url":       localURL,
		"sha256":           offer.Checksum,
		"signature":        offer.Signature,
		"release_notes":    offer.ReleaseNotes,
		"channel":          offer.Channel,
		"update_group":     offer.UpdateGroup,
	})
}

// handleChildReport records the child's install result; it surfaces in the
// next rollup so failures are visible at every level up to the updates server.
func (r *Relay) handleChildReport(w http.ResponseWriter, req *http.Request) {
	product := req.PathValue("product")
	var report UpdateReportRequest
	if err := json.NewDecoder(io.LimitReader(req.Body, 1<<20)).Decode(&report); err != nil {
		relayError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !r.authorizeChild(w, req, report.InstanceID) {
		return
	}

	r.mu.Lock()
	if child, ok := r.children[report.InstanceID]; ok {
		child.LastReport = &platformtypes.UpdateAttempt{
			FromVersion:   report.FromVersion,
			TargetVersion: report.ToVersion,
			Success:       report.Success,
			Error:         report.Error,
			Timestamp:     time.Now().UTC(),
		}
	}
	r.mu.Unlock()

	r.logger.Info("child update report",
		"instance_id", report.InstanceID,
		"product", product,
		"from", report.FromVersion,
		"to", report.ToVersion,
		"success", report.Success,
	)
	relayJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

// handleChildDownload serves an artifact from the verified pull-through cache,
// fetching and verifying it from the parent on first request.
func (r *Relay) handleChildDownload(w http.ResponseWriter, req *http.Request) {
	product := req.PathValue("product")
	version := req.PathValue("version")
	if !productNamePattern.MatchString(product) || strings.ContainsAny(version, "/\\") {
		relayError(w, http.StatusBadRequest, "invalid product or version")
		return
	}
	if strings.TrimSpace(req.Header.Get("X-License-Key")) == "" {
		relayError(w, http.StatusUnauthorized, "a child credential (X-License-Key) is required")
		return
	}

	meta, path, err := r.ensureCached(req.Context(), product, version)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			relayError(w, apiErr.StatusCode, apiErr.Message)
			return
		}
		r.logger.Error("relay pull-through failed", "product", product, "version", version, "error", err)
		relayError(w, http.StatusBadGateway, err.Error())
		return
	}

	file, err := os.Open(path)
	if err != nil {
		relayError(w, http.StatusInternalServerError, "cached artifact unavailable")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(path))
	w.Header().Set("X-Checksum-SHA256", meta.Checksum)
	if meta.Signature != "" {
		w.Header().Set("X-Signature-Ed25519", meta.Signature)
	}
	io.Copy(w, file)
}

// handleChildReleaseMeta proxies release metadata (checksum, signature, size)
// from the parent, so a child that is itself a relay can verify artifacts
// before caching. Without this route the cascade breaks past one relay hop.
func (r *Relay) handleChildReleaseMeta(w http.ResponseWriter, req *http.Request) {
	product := req.PathValue("product")
	version := req.PathValue("version")
	if !productNamePattern.MatchString(product) || strings.ContainsAny(version, "/\\") {
		relayError(w, http.StatusBadRequest, "invalid product or version")
		return
	}
	if strings.TrimSpace(req.Header.Get("X-License-Key")) == "" {
		relayError(w, http.StatusUnauthorized, "a child credential (X-License-Key) is required")
		return
	}

	meta, err := r.upstream.GetReleaseMeta(req.Context(), product, version)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			relayError(w, apiErr.StatusCode, apiErr.Message)
			return
		}
		relayError(w, http.StatusBadGateway, "upstream metadata fetch failed: "+err.Error())
		return
	}
	relayJSON(w, http.StatusOK, releaseMetaBody(meta))
}

// releaseMetaBody renders release metadata for children as a superset of the
// upstream object: the guide's canonical flat keys (product, size) are
// guaranteed for leaf agents, while the upstream field names (product_name,
// artifact_size, checksum, signature, ...) remain for downstream relays that
// parse the Release struct. Both consumers stay satisfied by one payload.
func releaseMetaBody(meta *platformtypes.Release) map[string]interface{} {
	body := map[string]interface{}{}
	if raw, err := json.Marshal(meta); err == nil {
		_ = json.Unmarshal(raw, &body)
	}
	body["product"] = meta.ProductName
	body["size"] = meta.ArtifactSize
	return body
}

// ensureCached returns release metadata and the local path of the verified
// artifact, downloading it from the parent if the cache misses.
func (r *Relay) ensureCached(
	ctx context.Context,
	product, version string,
) (*platformtypes.Release, string, error) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	meta, err := r.upstream.GetReleaseMeta(ctx, product, version)
	if err != nil {
		return nil, "", fmt.Errorf("fetch release metadata: %w", err)
	}

	// The cascade's integrity guarantee: verify the origin signature before
	// serving anything downstream.
	if r.publicKey != nil {
		if err := signing.Verify(r.publicKey, product, version, meta.Checksum, meta.Signature); err != nil {
			return nil, "", fmt.Errorf("refusing to relay %s %s: %w", product, version, err)
		}
	} else if r.config.Signing.Require {
		return nil, "", fmt.Errorf("signing.require is set but no public key is configured")
	}

	fileName := fmt.Sprintf("%s-%s.artifact", product, version)
	cachePath := filepath.Join(r.config.Relay.CacheDir, safeFileName(fileName))
	if info, err := os.Stat(cachePath); err == nil && info.Size() > 0 {
		return meta, cachePath, nil
	}

	downloadURL := fmt.Sprintf("/api/v1/releases/%s/%s/download", product, version)
	result, err := r.upstream.DownloadArtifact(
		ctx,
		downloadURL,
		meta.Checksum,
		r.config.Relay.CacheDir,
		fileName,
		r.config.Relay.MaxArtifactBytes,
	)
	if err != nil {
		return nil, "", fmt.Errorf("pull artifact from parent: %w", err)
	}
	r.logger.Info("artifact cached at relay",
		"product", product,
		"version", version,
		"bytes", result.Size,
		"sha256", result.Checksum,
	)
	return meta, result.Path, nil
}

// authorizeChild enforces credential presence and, for known children, the
// relay token issued at enrollment.
func (r *Relay) authorizeChild(w http.ResponseWriter, req *http.Request, instanceID string) bool {
	if strings.TrimSpace(req.Header.Get("X-License-Key")) == "" {
		r.guard.noteAuthFailure(req.RemoteAddr)
		relayError(w, http.StatusUnauthorized, "a child credential (X-License-Key) is required")
		return false
	}
	presented := strings.TrimSpace(req.Header.Get("X-Relay-Token"))
	r.mu.Lock()
	if child, ok := r.children[instanceID]; ok && child.Token != "" && presented != child.Token {
		r.mu.Unlock()
		r.guard.noteAuthFailure(req.RemoteAddr)
		relayError(w, http.StatusUnauthorized, "relay token mismatch for this instance_id")
		return false
	}
	r.mu.Unlock()
	r.guard.noteAuthSuccess(req.RemoteAddr)
	return true
}

// GuardStats exposes the port-protection counters for the upward heartbeat.
func (r *Relay) GuardStats() *platformtypes.RelayGuardStats {
	return r.guard.Stats()
}

func newRelayToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("rt_%d", time.Now().UnixNano())
	}
	return "rt_" + hex.EncodeToString(buf)
}

func relayJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func relayError(w http.ResponseWriter, status int, message string) {
	relayJSON(w, status, map[string]string{"error": message})
}
