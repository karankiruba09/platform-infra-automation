package checks

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"
)

var DefaultAddonsNamespaces = []string{
	"pinniped-concierge",
	"pinniped-supervisor",
	"cert-manager",
	"kapp-controller",
	"metrics-server",
	"antrea-system",
	"cilium",
	"external-dns",
	"projectcontour",
	"avi-system",
}

type Options struct {
	NamespacesInclude []string
	NamespacesExclude []string

	AddonsNamespaces []string

	CheckStorage   bool
	StorageClasses []string

	SmokeURL string

	CheckAirgap    bool
	AirgapEndpoint string
}

func Run(ctx context.Context, kc *kube.Client, opts Options) report.Report {
	rep := report.Report{
		Title:     "TCA/TKG Pre-Upgrade Health Report",
		Generated: time.Now(),
	}
	if kc != nil && kc.HasRawInfo && kc.Context != "" {
		rep.Context = kc.Context
	}

	results := []report.CheckResult{
		CheckAPIServerHealth(ctx, kc),
		CheckNodesAndVersionSkew(ctx, kc),
		CheckWorkloadReadiness(ctx, kc, opts.NamespacesInclude, opts.NamespacesExclude),
		CheckTKGManagementAwareness(ctx, kc),
		CheckTCASpecifics(ctx, kc, opts.CheckAirgap, opts.AirgapEndpoint),
		CheckPinnipedCredentialIssuer(ctx, kc),
		CheckAddonsNamespaces(ctx, kc, opts.AddonsNamespaces),
		CheckStorageSanity(ctx, kc, opts.CheckStorage, opts.StorageClasses),
		CheckIngressSmoke(ctx, opts.SmokeURL),
	}
	results = append(results, CheckUpgradeReadinessSummary(results))
	rep.Results = results

	// Ensure stable IDs and ordering in output.
	for i := range rep.Results {
		if rep.Results[i].ID == "" {
			rep.Results[i].ID = fmt.Sprintf("%d", i+1)
		}
	}

	return rep
}

func CheckUpgradeReadinessSummary(results []report.CheckResult) report.CheckResult {
	res := report.CheckResult{
		ID:       "J",
		Category: "Upgrade readiness summary",
		Status:   report.StatusPass,
	}

	var pass, warn, fail int
	next := map[string]struct{}{}
	for _, r := range results {
		switch r.Status {
		case report.StatusPass:
			pass++
		case report.StatusWarn:
			warn++
			for _, ns := range r.NextSteps {
				next[ns] = struct{}{}
			}
		case report.StatusFail:
			fail++
			for _, ns := range r.NextSteps {
				next[ns] = struct{}{}
			}
		}
		res.Status = report.WorstStatus(res.Status, r.Status)
	}

	res.Summary = fmt.Sprintf("PASS=%d WARN=%d FAIL=%d", pass, warn, fail)
	if len(next) > 0 {
		for cmd := range next {
			res.NextSteps = append(res.NextSteps, cmd)
		}
		sort.Strings(res.NextSteps)
	}
	return res
}
