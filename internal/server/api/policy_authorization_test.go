package api

import (
	"encoding/json"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/policyauth"
	"strings"
	"testing"
	"time"
)

func TestPolicyGrantStorageIsInstanceScopedAndExpires(t *testing.T) {
	store, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{storage: store}
	grant := policyauth.Envelope{Payload: policyauth.Payload{ExpiresAt: time.Now().Unix() + 60}}
	raw, _ := json.Marshal(grant)
	if _, err = store.Save("policy-authorizations", "3.3.152.32", policyGrantFile("testing"), strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	if len(s.policyGrant("testing", "3.3.152.32")) == 0 {
		t.Fatal("missing scoped grant")
	}
	if len(s.policyGrant("other", "3.3.152.32")) != 0 || len(s.policyGrant("testing", "3.3.152.33")) != 0 {
		t.Fatal("cross scope grant")
	}
	grant.Payload.ExpiresAt = time.Now().Unix() - 1
	raw, _ = json.Marshal(grant)
	store.Save("policy-authorizations", "3.3.152.32", policyGrantFile("testing"), strings.NewReader(string(raw)))
	if len(s.policyGrant("testing", "3.3.152.32")) != 0 {
		t.Fatal("expired grant delivered")
	}
}
