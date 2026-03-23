package checks

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"
)

func CheckIngressSmoke(ctx context.Context, url string) report.CheckResult {
	res := report.CheckResult{
		ID:       "I",
		Category: "Ingress/service smoke (optional)",
		Status:   report.StatusPass,
	}
	if url == "" {
		res.Summary = "Skipped (set --smoke-url)."
		return res
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Invalid smoke URL: %v", err)
		return res
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	lat := time.Since(start)
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Smoke GET failed: %v", err)
		res.Details = append(res.Details, fmt.Sprintf("url=%s latency=%s", url, lat))
		return res
	}
	_ = resp.Body.Close()

	res.Details = append(res.Details, fmt.Sprintf("url=%s status=%s latency=%s", url, resp.Status, lat))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		res.Status = report.StatusFail
		res.Summary = "Smoke URL returned non-2xx/3xx status."
		return res
	}
	res.Summary = "Smoke URL reachable."
	return res
}
