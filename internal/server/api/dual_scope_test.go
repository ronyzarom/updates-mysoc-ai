package api

import (
	"bytes"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/config"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDualQualificationRejectsSharedChannelsBeforePublication(t *testing.T) {
	for _, product := range []string{"mysoc", "siemcore", "swf"} {
		for _, channel := range []string{"", "stable", "alpha", "dual-alpha-other", "dual-alpha-" + product} {
			t.Run(product+"/"+channel, func(t *testing.T) {
				var body bytes.Buffer
				w := multipart.NewWriter(&body)
				for k, v := range map[string]string{"product": product, "version": "99.0.0.1", "channel": channel, "target_groups": "alpha", "artifact_variants": "invalid"} {
					if err := w.WriteField(k, v); err != nil {
						t.Fatal(err)
					}
				}
				w.Close()
				req := httptest.NewRequest("POST", "/api/v1/releases", &body)
				req.Header.Set("Content-Type", w.FormDataContentType())
				resp := httptest.NewRecorder()
				s := &Server{config: &config.Config{Server: config.ServerConfig{DualArtifactAlpha: true, DualArtifactChannelPrefix: "dual-alpha-"}}}
				s.handleUploadRelease(resp, req)
				want := "dual artifact qualification requires channel"
				if channel == "dual-alpha-"+product {
					want = "invalid artifact_variants metadata"
				}
				if resp.Code != 400 || !strings.Contains(resp.Body.String(), want) {
					t.Fatalf("%d %s", resp.Code, resp.Body.String())
				}
			})
		}
	}
}
