package checks

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"

	"k8s.io/client-go/rest"
)

func CheckAPIServerHealth(ctx context.Context, kc *kube.Client) report.CheckResult {
	res := report.CheckResult{
		ID:       "A",
		Category: "API server health",
		Status:   report.StatusPass,
		Summary:  "Checked /livez and /readyz (canonical Kubernetes health endpoints).",
		NextSteps: []string{
			"kubectl get --raw='/livez?verbose'",
			"kubectl get --raw='/readyz?verbose'",
		},
	}
	if kc == nil || kc.RESTConfig == nil {
		res.Status = report.StatusFail
		res.Summary = "Missing Kubernetes REST config."
		return res
	}

	rt, err := rest.TransportFor(kc.RESTConfig)
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Failed to build Kubernetes transport: %v", err)
		return res
	}

	base, err := url.Parse(kc.RESTConfig.Host)
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Failed to parse API server host: %v", err)
		return res
	}

	client := &http.Client{Transport: rt}

	livezCode, livezLatency, livezErr := doHTTPProbe(ctx, client, base, "/livez")
	readyzCode, readyzLatency, readyzErr := doHTTPProbe(ctx, client, base, "/readyz")

	res.Details = append(res.Details,
		fmt.Sprintf("/livez  status=%s latency=%s", formatHTTPStatus(livezCode, livezErr), livezLatency),
		fmt.Sprintf("/readyz status=%s latency=%s", formatHTTPStatus(readyzCode, readyzErr), readyzLatency),
	)

	if livezCode != http.StatusOK || readyzCode != http.StatusOK || livezErr != nil || readyzErr != nil {
		res.Status = report.StatusFail
		res.Summary = "API server health endpoint returned a non-200 status."
		return res
	}
	if livezLatency > 2*time.Second || readyzLatency > 2*time.Second {
		res.Status = report.StatusWarn
		res.Summary = "API server health endpoint latency exceeded 2s."
		return res
	}
	return res
}

func doHTTPProbe(ctx context.Context, client *http.Client, base *url.URL, path string) (int, time.Duration, error) {
	u := base.ResolveReference(&url.URL{Path: path})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, 0, err
	}

	start := time.Now()
	resp, err := client.Do(req)
	lat := time.Since(start)
	if err != nil {
		return 0, lat, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, lat, nil
}

func formatHTTPStatus(code int, err error) string {
	if err != nil {
		return fmt.Sprintf("error(%v)", err)
	}
	return fmt.Sprintf("%d", code)
}
