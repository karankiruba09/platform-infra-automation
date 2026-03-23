package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/checks"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"
)

func main() {
	var (
		kubeContext    = flag.String("kubecontext", "", "kubeconfig context name (optional)")
		timeoutSeconds = flag.Int("timeout-seconds", 60, "overall timeout in seconds")

		outputMD = flag.String("output-md", "", "write a Markdown report to this file (optional)")
		noColor  = flag.Bool("no-color", false, "disable ANSI colors")

		namespacesInclude = flag.String("namespaces-include", "", "comma-separated namespace allow-list for workload checks (optional)")
		namespacesExclude = flag.String("namespaces-exclude", "kube-system", "comma-separated namespace block-list for workload checks (optional)")

		addonsNamespaces = flag.String("addons-namespaces", strings.Join(checks.DefaultAddonsNamespaces, ","), "comma-separated namespaces to treat as critical add-ons")

		checkStorage   = flag.Bool("check-storage", false, "create and delete a tiny PVC in the default StorageClass (optional)")
		storageClasses = flag.String("storageclasses", "", "comma-separated StorageClass names to test when --check-storage is set (optional)")

		smokeURL = flag.String("smoke-url", "", "make a single GET request to this URL (optional)")

		checkAirgap    = flag.Bool("check-airgap", false, "check TLS and reachability for an air-gap registry endpoint using the TCA annotation CA cert (optional)")
		airgapEndpoint = flag.String("airgap-endpoint", "", "air-gap registry endpoint URL, e.g. https://registry.example.com/v2/ (required when --check-airgap)")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSeconds)*time.Second)
	defer cancel()

	kc, err := kube.NewClient(kube.Options{
		KubeContext: *kubeContext,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create kube client: %v\n", err)
		os.Exit(2)
	}

	opts := checks.Options{
		NamespacesInclude: splitCSV(*namespacesInclude),
		NamespacesExclude: splitCSV(*namespacesExclude),
		AddonsNamespaces:  splitCSV(*addonsNamespaces),

		CheckStorage:   *checkStorage,
		StorageClasses: splitCSV(*storageClasses),

		SmokeURL: *smokeURL,

		CheckAirgap:    *checkAirgap,
		AirgapEndpoint: strings.TrimSpace(*airgapEndpoint),
	}

	rep := checks.Run(ctx, kc, opts)

	tty := report.StdoutIsTTY() && !*noColor
	report.NewPrinter(os.Stdout, report.PrinterOptions{Color: tty}).Print(rep)

	if *outputMD != "" {
		if err := os.WriteFile(*outputMD, []byte(report.RenderMarkdown(rep)), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write markdown output: %v\n", err)
			os.Exit(2)
		}
	}

	if rep.HasFail() {
		os.Exit(2)
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
