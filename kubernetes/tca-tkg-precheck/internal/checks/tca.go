package checks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	airgapCertAnnotation = "telco.vmware.com/airgap-ca-cert"
)

func CheckTCASpecifics(ctx context.Context, kc *kube.Client, checkAirgap bool, airgapEndpoint string) report.CheckResult {
	res := report.CheckResult{
		ID:       "E",
		Category: "TCA specifics",
		Status:   report.StatusPass,
		NextSteps: []string{
			"kubectl get tcakubernetesclusters -A",
		},
	}
	if kc == nil || kc.Dynamic == nil || kc.Discovery == nil {
		res.Status = report.StatusFail
		res.Summary = "Missing Kubernetes dynamic/discovery client."
		return res
	}

	refs, err := findGVRsByKind(ctx, kc, "TcaKubernetesCluster")
	if err != nil {
		res.Status = report.StatusWarn
		res.Summary = fmt.Sprintf("Failed to discover TcaKubernetesCluster API: %v", err)
		return res
	}
	if len(refs) == 0 {
		res.Summary = "TcaKubernetesCluster CRD not detected; skipping TCA-specific checks."
		return res
	}

	list, err := kc.Dynamic.Resource(refs[0]).List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusWarn
		res.Summary = fmt.Sprintf("Failed to list TcaKubernetesCluster: %v", err)
		return res
	}
	if len(list.Items) == 0 {
		res.Summary = "No TcaKubernetesCluster objects found."
		return res
	}

	var foundCert bool
	var certsForAirgap []*x509.Certificate
	for i := range list.Items {
		obj := &list.Items[i]
		ann := obj.GetAnnotations()
		b64 := strings.TrimSpace(ann[airgapCertAnnotation])
		if b64 == "" {
			continue
		}
		foundCert = true
		certs, err := parseCertsFromB64(b64)
		if err != nil {
			res.Status = report.StatusFail
			res.Summary = fmt.Sprintf("Failed to decode %s on %s/%s: %v", airgapCertAnnotation, obj.GetNamespace(), obj.GetName(), err)
			return res
		}
		if len(certs) == 0 {
			res.Status = report.StatusFail
			res.Summary = fmt.Sprintf("No certificates found after decoding %s on %s/%s.", airgapCertAnnotation, obj.GetNamespace(), obj.GetName())
			return res
		}
		c := certs[0]
		res.Details = append(res.Details,
			fmt.Sprintf("%s/%s %s subject=%s notAfter=%s", obj.GetNamespace(), obj.GetName(), airgapCertAnnotation, c.Subject.String(), c.NotAfter.Format(time.RFC3339)),
		)
		now := time.Now()
		if now.After(c.NotAfter) {
			res.Status = report.StatusFail
			res.Summary = fmt.Sprintf("Air-gap CA certificate is expired (%s).", c.NotAfter.Format(time.RFC3339))
			res.NextSteps = append(res.NextSteps,
				fmt.Sprintf("kubectl get %s -n %s %s -o yaml", refs[0].Resource, obj.GetNamespace(), obj.GetName()),
			)
			return res
		}
		if certsForAirgap == nil {
			certsForAirgap = certs
		}
	}

	if checkAirgap {
		if strings.TrimSpace(airgapEndpoint) == "" {
			res.Status = report.StatusFail
			res.Summary = "--check-airgap was set but --airgap-endpoint was empty."
			return res
		}
		if !foundCert {
			res.Status = report.StatusFail
			res.Summary = fmt.Sprintf("--check-airgap was set but no %s annotation was found.", airgapCertAnnotation)
			return res
		}
		if err := checkAirgapEndpoint(ctx, airgapEndpoint, certsForAirgap); err != nil {
			res.Status = report.StatusFail
			res.Summary = fmt.Sprintf("Air-gap endpoint check failed: %v", err)
			return res
		}
		res.Details = append(res.Details, fmt.Sprintf("Air-gap endpoint reachable with provided CA: %s", airgapEndpoint))
	}

	if !foundCert {
		res.Summary = fmt.Sprintf("TcaKubernetesCluster found (%d) but %s annotation not present.", len(list.Items), airgapCertAnnotation)
		return res
	}
	res.Summary = "TCA custom resources inspected."
	return res
}

func parseCertsFromB64(b64 string) ([]*x509.Certificate, error) {
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(b64)
		if err != nil {
			return nil, err
		}
	}

	// Most commonly the decoded bytes are PEM. Handle multiple certs.
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
	if len(certs) > 0 {
		return certs, nil
	}

	// Fallback: assume a single DER certificate.
	c, err := x509.ParseCertificate(decoded)
	if err != nil {
		return nil, err
	}
	return []*x509.Certificate{c}, nil
}

func checkAirgapEndpoint(ctx context.Context, endpoint string, certs []*x509.Certificate) error {
	pool := x509.NewCertPool()
	for _, c := range certs {
		pool.AddCert(c)
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    pool,
		},
	}
	client := &http.Client{Transport: tr}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		// Some registries don't support HEAD; try GET.
		req2, err2 := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err2 != nil {
			return err
		}
		resp2, err2 := client.Do(req2)
		if err2 != nil {
			return err
		}
		_ = resp2.Body.Close()
		if resp2.StatusCode < 200 || resp2.StatusCode >= 500 {
			return fmt.Errorf("unexpected HTTP status: %s", resp2.Status)
		}
		return nil
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 500 {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}
	return nil
}

// findGVRsByKind finds matching resources for a Kind using discovery. It is used for CRDs
// where the exact group/version differs across product releases.
func findGVRsByKind(ctx context.Context, kc *kube.Client, kind string) ([]schema.GroupVersionResource, error) {
	// ServerPreferredResources can partially fail; treat partial discovery as usable.
	lists, err := kc.Discovery.ServerPreferredResources()
	if err != nil && !isDiscoveryPartialFailure(err) {
		return nil, err
	}
	var out []schema.GroupVersionResource
	for _, rl := range lists {
		gv, err := schema.ParseGroupVersion(rl.GroupVersion)
		if err != nil {
			continue
		}
		for _, r := range rl.APIResources {
			if r.Kind != kind {
				continue
			}
			if strings.Contains(r.Name, "/") {
				// Skip subresources.
				continue
			}
			out = append(out, schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: r.Name})
		}
	}
	return out, nil
}

func isDiscoveryPartialFailure(err error) bool {
	// discovery.IsGroupDiscoveryFailedError is in client-go, but avoid importing it just for this.
	// The error type string is stable enough for best-effort behavior.
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "unable to retrieve the complete list of server APIs")
}
