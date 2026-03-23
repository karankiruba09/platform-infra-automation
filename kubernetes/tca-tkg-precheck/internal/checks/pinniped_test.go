package checks

import (
	"encoding/base64"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEvalCredentialIssuer_SuccessStrategy(t *testing.T) {
	_, pemBytes := makeTestCert(t, time.Now().Add(24*time.Hour))
	caB64 := base64.StdEncoding.EncodeToString(pemBytes)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"strategies": []any{
				map[string]any{
					"type":   "TokenCredentialRequestAPI",
					"status": "Success",
					"frontend": map[string]any{
						"tokenCredentialRequestInfo": map[string]any{
							"server":                   "https://concierge.example.com",
							"certificateAuthorityData": caB64,
						},
					},
				},
			},
		},
	}}

	healthy, endpoint, caNotAfter, reason := evalCredentialIssuer(obj)
	if !healthy {
		t.Fatalf("expected healthy=true, got false (reason=%q)", reason)
	}
	if endpoint != "https://concierge.example.com" {
		t.Fatalf("unexpected endpoint: %q", endpoint)
	}
	if caNotAfter == nil {
		t.Fatalf("expected caNotAfter to be set")
	}
}

func TestEvalCredentialIssuer_ErrorStrategy(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"strategies": []any{
				map[string]any{
					"type":    "TokenCredentialRequestAPI",
					"status":  "Error",
					"reason":  "SomeError",
					"message": "something failed",
				},
			},
		},
	}}

	healthy, endpoint, caNotAfter, reason := evalCredentialIssuer(obj)
	if healthy {
		t.Fatalf("expected healthy=false, got true")
	}
	if endpoint != "" {
		t.Fatalf("expected empty endpoint, got %q", endpoint)
	}
	if caNotAfter != nil {
		t.Fatalf("expected nil caNotAfter")
	}
	if reason == "" {
		t.Fatalf("expected non-empty reason")
	}
}
