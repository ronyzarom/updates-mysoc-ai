package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestInstallationTypesNeedsNoConfiguration(t *testing.T) {
	root := newRootCommand(&options{})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--config", "/does-not-exist", "installation-types"})
	if e := root.Execute(); e != nil {
		t.Fatal(e)
	}
	var response struct {
		Schema int      `json:"schema"`
		Types  []string `json:"server_types"`
		Ready  bool     `json:"lifecycle_ready"`
	}
	if e := json.Unmarshal(out.Bytes(), &response); e != nil {
		t.Fatal(e)
	}
	if response.Schema != 1 || len(response.Types) != 5 || response.Ready {
		t.Fatal(response)
	}
}
