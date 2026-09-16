package policyauth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Operation is the serialized policy-maintenance step. The caller must hold
// its product cycle lock and exclude any second updater process. Root verifies
// its own current policy independently; no product pointer/version is changed.
type Operation struct {
	RequestPath string
	PublicKey   ed25519.PublicKey
	Expected    Expected
	Invoke      func(context.Context) ([]byte, error)
}

func (o Operation) Run(ctx context.Context, raw []byte, now time.Time) (err error) {
	if _, err = Verify(raw, o.PublicKey, o.Expected, now); err != nil {
		return err
	}
	if o.Invoke == nil || !filepath.IsAbs(o.RequestPath) {
		return errors.New("policy consumer unavailable")
	}

	if st, statErr := os.Lstat(o.RequestPath); statErr == nil {
		if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return errors.New("unsafe pending policy request")
		}
		previous, readErr := os.ReadFile(o.RequestPath)
		if readErr != nil || !bytes.Equal(previous, raw) {
			return errors.New("different pending policy request requires reconciliation")
		}
		// Exact same signed request may retry its root-side prepared/committed receipt.
		if err = os.Remove(o.RequestPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}

	// Exclusive creation prevents overwrite of a pending request from another process.
	f, err := os.OpenFile(o.RequestPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := f.Close()
		_ = closeErr
		removeErr := os.Remove(o.RequestPath)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, removeErr)
		}
	}()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	output, err := o.Invoke(ctx)
	if err != nil {
		return fmt.Errorf("policy consumer: %w", err)
	}
	const prefix = "MYSOC_POLICY_RESULT_V1:"
	line := strings.TrimSpace(string(output))
	if !strings.HasPrefix(line, prefix) {
		return errors.New("missing policy receipt")
	}
	var result struct {
		RequestSHA256 string `json:"request_sha256"`
		PolicySHA256  string `json:"policy_sha256"`
		Status        string `json:"status"`
	}
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimPrefix(line, prefix)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&result); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("trailing policy receipt")
	}
	sum := sha256.Sum256(raw)
	if result.Status != "applied" || result.RequestSHA256 != hex.EncodeToString(sum[:]) || !hex64.MatchString(result.PolicySHA256) {
		return errors.New("policy receipt mismatch")
	}
	return nil
}
