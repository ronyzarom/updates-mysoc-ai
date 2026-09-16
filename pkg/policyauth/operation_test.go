package policyauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOperationReceiptFailureRetryAndNoInvokeOnInvalidGrant(t *testing.T) {
	p, w, pub, priv, now := fixture(t)
	envelope, _ := Sign(p, priv)
	raw, _ := json.Marshal(envelope)
	sum := sha256.Sum256(raw)
	for _, scenario := range []string{"success", "receipt-mismatch", "consumer-failure", "interrupted-retry", "different-pending", "invalid-grant"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "request.json")
			called := 0
			if scenario == "interrupted-retry" {
				os.WriteFile(path, raw, 0600)
			}
			if scenario == "different-pending" {
				os.WriteFile(path, []byte("different"), 0600)
			}
			op := Operation{RequestPath: path, PublicKey: pub, Expected: w, Invoke: func(context.Context) ([]byte, error) {
				called++
				staged, err := os.ReadFile(path)
				if err != nil || string(staged) != string(raw) {
					t.Fatal("incorrect staging")
				}
				if scenario == "consumer-failure" {
					return nil, errors.New("timeout")
				}
				hash := hex.EncodeToString(sum[:])
				if scenario == "receipt-mismatch" {
					hash = "wrong"
				}
				return []byte(`MYSOC_POLICY_RESULT_V1:{"request_sha256":"` + hash + `","policy_sha256":"` + p.ArtifactSHA256 + `","status":"applied"}`), nil
			}}
			stamp := now
			if scenario == "invalid-grant" {
				stamp = now.Add(time.Hour)
			}
			err := op.Run(context.Background(), raw, stamp)
			success := scenario == "success" || scenario == "interrupted-retry"
			if (err == nil) != success {
				t.Fatalf("unexpected result %v", err)
			}
			if scenario == "different-pending" || scenario == "invalid-grant" {
				if called != 0 {
					t.Fatal("invoked invalid request")
				}
			} else if _, err = os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("request not cleaned up")
			}
		})
	}
}
