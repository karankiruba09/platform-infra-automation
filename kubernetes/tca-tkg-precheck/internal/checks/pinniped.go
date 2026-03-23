package checks

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func CheckPinnipedCredentialIssuer(ctx context.Context, kc *kube.Client) report.CheckResult {
	res := report.CheckResult{
		ID:       "F",
		Category: "Pinniped status",
		Status:   report.StatusPass,
		NextSteps: []string{
			"kubectl get credentialissuers -o wide",
			"kubectl get pods -n pinniped-concierge",
		},
	}
	if kc == nil || kc.Dynamic == nil || kc.Discovery == nil {
		res.Status = report.StatusFail
		res.Summary = "Missing Kubernetes dynamic/discovery client."
		return res
	}

	gvrs, err := findPinnipedCredentialIssuerGVRs(ctx, kc)
	if err != nil {
		res.Status = report.StatusWarn
		res.Summary = fmt.Sprintf("Failed to discover CredentialIssuer API: %v", err)
		return res
	}
	if len(gvrs) == 0 {
		res.Status = report.StatusWarn
		res.Summary = "CredentialIssuer CRD not detected; skipping Pinniped health."
		return res
	}

	ciList, err := kc.Dynamic.Resource(gvrs[0]).List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusWarn
		res.Summary = fmt.Sprintf("Failed to list CredentialIssuer: %v", err)
		return res
	}
	if len(ciList.Items) == 0 {
		res.Status = report.StatusWarn
		res.Summary = "No CredentialIssuer objects found."
		return res
	}

	var lines []string
	allHealthy := true
	for i := range ciList.Items {
		obj := &ciList.Items[i]
		name := obj.GetName()
		healthy, endpoint, caNotAfter, reason := evalCredentialIssuer(obj)
		if !healthy {
			allHealthy = false
		}
		line := fmt.Sprintf("%s healthy=%t endpoint=%s", name, healthy, report.SanitizeOneLine(orDash(endpoint)))
		if caNotAfter != nil {
			line += " caNotAfter=" + caNotAfter.Format(time.RFC3339)
		}
		if reason != "" {
			line += " reason=" + report.SanitizeOneLine(reason)
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	res.Details = append(res.Details, "CredentialIssuer(s): "+strings.Join(truncate(lines, 10), " | "))

	if !allHealthy {
		res.Status = report.StatusFail
		res.Summary = "One or more CredentialIssuer strategies are not healthy."
		res.NextSteps = append(res.NextSteps,
			"kubectl get credentialissuers -o yaml",
			"kubectl logs -n pinniped-concierge deploy/pinniped-concierge",
		)
		return res
	}
	res.Summary = "CredentialIssuer reports at least one Success strategy."
	return res
}

func findPinnipedCredentialIssuerGVRs(ctx context.Context, kc *kube.Client) ([]schema.GroupVersionResource, error) {
	gvrs, err := findGVRsByKind(ctx, kc, "CredentialIssuer")
	if err != nil {
		return nil, err
	}
	// Prefer Concierge group if present.
	sort.Slice(gvrs, func(i, j int) bool {
		a := gvrs[i].Group
		b := gvrs[j].Group
		ai := strings.Contains(a, "concierge.pinniped.dev")
		bi := strings.Contains(b, "concierge.pinniped.dev")
		if ai != bi {
			return ai
		}
		return a < b
	})
	return gvrs, nil
}

func evalCredentialIssuer(obj *unstructured.Unstructured) (healthy bool, endpoint string, caNotAfter *time.Time, reason string) {
	strats, found, _ := unstructured.NestedSlice(obj.Object, "status", "strategies")
	if !found || len(strats) == 0 {
		return false, "", nil, "no status.strategies"
	}

	var anySuccess bool
	var lastMsg string
	for _, s := range strats {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		status, _ := m["status"].(string) // StrategyStatus: Success|Error
		reasonVal, _ := m["reason"].(string)
		msg, _ := m["message"].(string)
		if msg != "" {
			lastMsg = msg
		}
		if status == "Success" {
			anySuccess = true
			ep, ca := credentialIssuerFrontendInfo(m)
			if endpoint == "" {
				endpoint = ep
			}
			if caNotAfter == nil && ca != "" {
				if na := firstCertNotAfter(ca); na != nil {
					caNotAfter = na
				}
			}
		} else if status == "Error" && reason == "" && reasonVal != "" {
			reason = reasonVal
		}
	}
	if !anySuccess {
		if reason == "" && lastMsg != "" {
			reason = lastMsg
		}
		return false, "", nil, reason
	}
	return true, endpoint, caNotAfter, reason
}

func credentialIssuerFrontendInfo(strategy map[string]any) (endpoint string, caBundleB64 string) {
	frontend, ok := strategy["frontend"].(map[string]any)
	if !ok {
		return "", ""
	}
	// TokenCredentialRequest API (Concierge)
	if tcr, ok := frontend["tokenCredentialRequestInfo"].(map[string]any); ok {
		ep, _ := tcr["server"].(string)
		ca, _ := tcr["certificateAuthorityData"].(string)
		return ep, ca
	}
	// Impersonation proxy (Concierge)
	if ip, ok := frontend["impersonationProxyInfo"].(map[string]any); ok {
		ep, _ := ip["endpoint"].(string)
		ca, _ := ip["certificateAuthorityData"].(string)
		return ep, ca
	}
	return "", ""
}

func firstCertNotAfter(caBundleB64 string) *time.Time {
	certs, err := parsePEMCertsFromB64(caBundleB64)
	if err != nil || len(certs) == 0 {
		return nil
	}
	na := certs[0].NotAfter
	return &na
}

func parsePEMCertsFromB64(b64 string) ([]*x509.Certificate, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(b64))
		if err != nil {
			return nil, err
		}
	}
	var certs []*x509.Certificate
	rest := decoded
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("no PEM certificates found")
	}
	return certs, nil
}
