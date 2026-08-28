package updatersim

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// gzipRequestThreshold is the JSON body size above which the client gzips the
// request. Below it, gzip framing overhead outweighs the savings.
const gzipRequestThreshold = 4096

var (
	// ErrChecksumMismatch indicates that downloaded bytes do not match metadata.
	ErrChecksumMismatch = errors.New("artifact checksum mismatch")
	// ErrMissingChecksum indicates that no trusted digest accompanied a download.
	ErrMissingChecksum = errors.New("artifact checksum is required")
	// ErrExternalDownload indicates an unapproved cross-origin artifact URL.
	ErrExternalDownload = errors.New("external artifact download is not allowed")
)

// APIError is a non-success response from the Updates Server.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("updates server returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("updates server returned HTTP %d: %s", e.StatusCode, e.Message)
}

// HeartbeatResponse is the current heartbeat response contract.
type HeartbeatResponse struct {
	Status  string                      `json:"status"`
	Updates []platformtypes.ReleaseInfo `json:"updates"`
	// ReportedNodes echoes how many rollup nodes the server ingested.
	ReportedNodes int `json:"reported_nodes,omitempty"`
	// RelayToken is issued by a relay parent on first contact; the child
	// presents it on every subsequent request to that relay.
	RelayToken string `json:"relay_token,omitempty"`
	// AckCursor acknowledges the highest delta sequence the parent durably
	// ingested (Fleet Scalability 1.12). The sender prunes acked, unchanged
	// entries from its forwarding queue. Zero means "nothing acked".
	AckCursor uint64 `json:"ack_cursor,omitempty"`
	// HeartbeatIntervalSeconds, when non-zero, hints the interval the parent
	// wants this child to heartbeat at (larger at scale). Advisory only.
	HeartbeatIntervalSeconds int `json:"heartbeat_interval_seconds,omitempty"`
}

// UpdateCheckRequest is the current group-aware update-check request. ProductTier
// and ParentInstanceID carry the self-reported product hierarchy; servers that
// do not understand them ignore the extra fields.
type UpdateCheckRequest struct {
	InstanceID       string `json:"instance_id"`
	CurrentVersion   string `json:"current_version"`
	UpdaterVersion   string `json:"updater_version"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	Hostname         string `json:"hostname"`
	Channel          string `json:"channel"`
	ProductTier      string `json:"product_tier,omitempty"`
	ParentInstanceID string `json:"parent_instance_id,omitempty"`
}

// UpdateCheckResponse is the current group-aware update-check response.
type UpdateCheckResponse struct {
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion  string `json:"current_version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	DownloadURL     string `json:"download_url,omitempty"`
	UpdateURL       string `json:"update_url,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
	Signature       string `json:"signature,omitempty"` // base64 ed25519 release signature
	ReleaseNotes    string `json:"release_notes,omitempty"`
	Channel         string `json:"channel,omitempty"`
	UpdateGroup     string `json:"update_group,omitempty"`
	AutoUpdate      *bool  `json:"auto_update,omitempty"`
}

// UpdateReportRequest is the current update-result request. Kind and Stage are
// optional monitoring fields that let the server distinguish single-artifact
// updates from manifest reconciliation and record the stage a failure occurred
// in. Servers that do not understand them ignore the extra fields.
type UpdateReportRequest struct {
	InstanceID  string `json:"instance_id"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Stage       string `json:"stage,omitempty"`
}

// UpdateOffer normalizes the current policy and legacy response formats.
type UpdateOffer struct {
	Product         string
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	DownloadURL     string
	Checksum        string
	Signature       string
	ReleaseNotes    string
	Channel         string
	UpdateGroup     string
	Source          string
}

// DownloadResult describes a verified artifact saved by the simulator.
type DownloadResult struct {
	Path     string
	Checksum string
	Size     int64
}

// Client communicates with the current Updates Server API.
type Client struct {
	baseURL                *url.URL
	httpClient             *http.Client
	licenseKey             string
	apiKey                 string
	relayToken             string
	maxResponseBytes       int64
	allowExternalDownloads bool
}

// NewClient creates an Updates Server client.
func NewClient(cfg ServerConfig) (*Client, error) {
	baseURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}

	client := &Client{
		baseURL:                baseURL,
		licenseKey:             cfg.LicenseKey,
		apiKey:                 cfg.APIKey,
		maxResponseBytes:       cfg.MaxResponseBytes,
		allowExternalDownloads: cfg.AllowExternalDownloads,
	}
	client.httpClient = &http.Client{
		Timeout: cfg.Timeout.Duration,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if !sameOrigin(baseURL, req.URL) {
				req.Header.Del("X-API-Key")
				req.Header.Del("X-License-Key")
				req.Header.Del("X-Relay-Token")
				if !cfg.AllowExternalDownloads {
					return ErrExternalDownload
				}
			}
			return nil
		},
	}
	if cfg.CAFile != "" {
		transport, err := transportWithCA(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		client.httpClient.Transport = transport
	}
	return client, nil
}

// transportWithCA builds a transport that trusts exactly the given PEM bundle
// for TLS verification. Used to pin a cascade relay's self-provisioned
// certificate instead of the system roots.
func transportWithCA(caFile string) (*http.Transport, error) {
	pemData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read ca_file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("ca_file %s contains no usable PEM certificates", caFile)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}
	return transport, nil
}

// SetAPIKey updates the credential returned by enrollment.
func (c *Client) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// SetRelayToken sets the token a relay parent issued to this node.
func (c *Client) SetRelayToken(token string) {
	c.relayToken = token
}

// GetReleaseMeta fetches release metadata (checksum, signature, size) for one
// version. Used by relays to verify artifacts before caching.
func (c *Client) GetReleaseMeta(
	ctx context.Context,
	product, version string,
) (*platformtypes.Release, error) {
	if !productNamePattern.MatchString(product) {
		return nil, fmt.Errorf("invalid product name %q", product)
	}
	path := "/api/v1/releases/" + url.PathEscape(product) + "/" + url.PathEscape(version)
	var release platformtypes.Release
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

// ActivateLicense enrolls a simulator instance. This mutates server state.
func (c *Client) ActivateLicense(
	ctx context.Context,
	request platformtypes.LicenseActivationRequest,
) (*platformtypes.LicenseActivationResponse, error) {
	var response platformtypes.LicenseActivationResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/license/activate", request, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return &response, fmt.Errorf("license activation failed: %s", response.Error)
	}
	return &response, nil
}

// SendHeartbeat posts a heartbeat and parses update offers returned by the server.
func (c *Client) SendHeartbeat(
	ctx context.Context,
	heartbeat platformtypes.Heartbeat,
) (*HeartbeatResponse, error) {
	var response HeartbeatResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/heartbeat", heartbeat, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// CheckUpdate calls the preferred group-aware policy endpoint.
func (c *Client) CheckUpdate(
	ctx context.Context,
	product string,
	request UpdateCheckRequest,
) (*UpdateOffer, error) {
	if !productNamePattern.MatchString(product) {
		return nil, fmt.Errorf("invalid product name %q", product)
	}

	var response UpdateCheckResponse
	path := "/api/v1/updates/" + url.PathEscape(product) + "/check"
	if err := c.doJSON(ctx, http.MethodPost, path, request, &response); err != nil {
		return nil, err
	}

	downloadURL := response.DownloadURL
	if downloadURL == "" {
		downloadURL = response.UpdateURL
	}
	return &UpdateOffer{
		Product:         product,
		CurrentVersion:  request.CurrentVersion,
		LatestVersion:   response.LatestVersion,
		UpdateAvailable: response.UpdateAvailable,
		DownloadURL:     downloadURL,
		Checksum:        normalizeChecksum(response.SHA256),
		Signature:       response.Signature,
		ReleaseNotes:    response.ReleaseNotes,
		Channel:         response.Channel,
		UpdateGroup:     response.UpdateGroup,
		Source:          "policy",
	}, nil
}

// CheckLatest calls the legacy group-unaware release endpoint.
func (c *Client) CheckLatest(
	ctx context.Context,
	product, channel, currentVersion string,
) (*UpdateOffer, error) {
	if !productNamePattern.MatchString(product) {
		return nil, fmt.Errorf("invalid product name %q", product)
	}

	query := url.Values{}
	query.Set("channel", channel)
	query.Set("current_version", currentVersion)
	path := "/api/v1/releases/" + url.PathEscape(product) + "/latest?" + query.Encode()

	var response platformtypes.ReleaseInfo
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return &UpdateOffer{
		Product:         response.Product,
		CurrentVersion:  response.CurrentVersion,
		LatestVersion:   response.LatestVersion,
		UpdateAvailable: response.UpdateAvailable,
		DownloadURL:     response.DownloadURL,
		Checksum:        normalizeChecksum(response.Checksum),
		Signature:       response.Signature,
		ReleaseNotes:    response.ReleaseNotes,
		Channel:         response.Channel,
		Source:          "legacy",
	}, nil
}

// DownloadArtifact downloads and verifies an artifact without executing it.
func (c *Client) DownloadArtifact(
	ctx context.Context,
	rawURL, expectedChecksum, destinationDir, fileName string,
	maxBytes int64,
) (*DownloadResult, error) {
	downloadURL, err := c.resolveURL(rawURL)
	if err != nil {
		return nil, err
	}
	external := !sameOrigin(c.baseURL, downloadURL)
	if external && !c.allowExternalDownloads {
		return nil, ErrExternalDownload
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create artifact request: %w", err)
	}
	if !external {
		c.addAuthHeaders(request)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download artifact: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		data, readErr := readBounded(response.Body, c.maxResponseBytes)
		if readErr != nil {
			return nil, fmt.Errorf("read artifact error response: %w", readErr)
		}
		return nil, apiError(response.StatusCode, data)
	}

	expectedChecksum = normalizeChecksum(expectedChecksum)
	headerChecksum := normalizeChecksum(response.Header.Get("X-Checksum-SHA256"))
	if expectedChecksum == "" && headerChecksum == "" {
		return nil, ErrMissingChecksum
	}
	if expectedChecksum != "" && headerChecksum != "" && expectedChecksum != headerChecksum {
		return nil, fmt.Errorf("%w: metadata and response header disagree", ErrChecksumMismatch)
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("maximum artifact size must be positive")
	}

	if err := os.MkdirAll(destinationDir, 0700); err != nil {
		return nil, fmt.Errorf("create artifact directory: %w", err)
	}
	safeName := safeFileName(fileName)
	tempFile, err := os.CreateTemp(destinationDir, "."+safeName+".tmp-*")
	if err != nil {
		return nil, fmt.Errorf("create artifact temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if err := tempFile.Chmod(0600); err != nil {
		tempFile.Close()
		return nil, fmt.Errorf("protect artifact temp file: %w", err)
	}

	hasher := sha256.New()
	written, copyErr := io.Copy(
		io.MultiWriter(tempFile, hasher),
		io.LimitReader(response.Body, maxBytes+1),
	)
	if copyErr != nil {
		tempFile.Close()
		return nil, fmt.Errorf("write artifact: %w", copyErr)
	}
	if written > maxBytes {
		tempFile.Close()
		return nil, fmt.Errorf("artifact exceeds maximum size of %d bytes", maxBytes)
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return nil, fmt.Errorf("sync artifact: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("close artifact: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if expectedChecksum != "" && actualChecksum != expectedChecksum {
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedChecksum, actualChecksum)
	}
	if headerChecksum != "" && actualChecksum != headerChecksum {
		return nil, fmt.Errorf("%w: expected header %s, got %s", ErrChecksumMismatch, headerChecksum, actualChecksum)
	}

	destinationPath := filepath.Join(destinationDir, safeName)
	if err := os.Rename(tempPath, destinationPath); err != nil {
		return nil, fmt.Errorf("commit verified artifact: %w", err)
	}
	return &DownloadResult{
		Path:     destinationPath,
		Checksum: actualChecksum,
		Size:     written,
	}, nil
}

// ReportUpdate reports a simulated result to the current acknowledgement endpoint.
func (c *Client) ReportUpdate(
	ctx context.Context,
	product string,
	request UpdateReportRequest,
) error {
	if !productNamePattern.MatchString(product) {
		return fmt.Errorf("invalid product name %q", product)
	}
	path := "/api/v1/updates/" + url.PathEscape(product) + "/report"
	var response struct {
		Status string `json:"status"`
	}
	return c.doJSON(ctx, http.MethodPost, path, request, &response)
}

func (c *Client) doJSON(
	ctx context.Context,
	method, path string,
	requestBody, responseBody interface{},
) error {
	target, err := c.resolveURL(path)
	if err != nil {
		return err
	}

	var body io.Reader
	gzipped := false
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		// Compress large bodies (the cascade rollup heartbeat dominates and
		// is highly compressible ~10x). Small bodies skip it — gzip framing
		// would only add overhead. Receivers advertise no requirement; they
		// simply honor Content-Encoding, and pre-1.12 receivers never see a
		// gzipped body because they run older clients.
		if len(data) >= gzipRequestThreshold {
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			if _, err := zw.Write(data); err != nil {
				return fmt.Errorf("gzip request: %w", err)
			}
			if err := zw.Close(); err != nil {
				return fmt.Errorf("gzip request: %w", err)
			}
			body = &buf
			gzipped = true
		} else {
			body = bytes.NewReader(data)
		}
	}

	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
		if gzipped {
			request.Header.Set("Content-Encoding", "gzip")
		}
	}
	request.Header.Set("Accept", "application/json")
	c.addAuthHeaders(request)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call updates server: %w", err)
	}
	defer response.Body.Close()

	data, err := readBounded(response.Body, c.maxResponseBytes)
	if err != nil {
		return fmt.Errorf("read updates server response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return apiError(response.StatusCode, data)
	}
	if responseBody == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decode updates server response: %w", err)
	}
	return nil
}

func (c *Client) resolveURL(rawURL string) (*url.URL, error) {
	reference, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse updates server URL: %w", err)
	}
	resolved := c.baseURL.ResolveReference(reference)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return nil, fmt.Errorf("resolved URL must use http or https")
	}
	return resolved, nil
}

func (c *Client) addAuthHeaders(request *http.Request) {
	if c.apiKey != "" {
		request.Header.Set("X-API-Key", c.apiKey)
	}
	if c.licenseKey != "" {
		request.Header.Set("X-License-Key", c.licenseKey)
	}
	if c.relayToken != "" {
		request.Header.Set("X-Relay-Token", c.relayToken)
	}
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("response exceeds maximum size of %d bytes", maximum)
	}
	return data, nil
}

func apiError(statusCode int, data []byte) error {
	var response struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(data, &response)
	message := strings.TrimSpace(response.Error)
	if message == "" {
		message = strings.TrimSpace(string(data))
	}
	if len(message) > 512 {
		message = message[:512] + "..."
	}
	return &APIError{StatusCode: statusCode, Message: message}
}

func normalizeChecksum(checksum string) string {
	checksum = strings.ToLower(strings.TrimSpace(checksum))
	checksum = strings.TrimPrefix(checksum, "sha256:")
	return checksum
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Host, right.Host)
}

func safeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		return "artifact.bin"
	}

	var builder strings.Builder
	for _, char := range name {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '.', char == '-', char == '_':
			builder.WriteRune(char)
		default:
			builder.WriteRune('_')
		}
	}
	if builder.Len() == 0 {
		return "artifact.bin"
	}
	return builder.String()
}
