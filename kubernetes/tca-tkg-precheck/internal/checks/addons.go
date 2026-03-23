package checks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"

	admv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CheckAddonsNamespaces(ctx context.Context, kc *kube.Client, namespaces []string) report.CheckResult {
	res := report.CheckResult{
		ID:       "G",
		Category: "Add-ons in system namespaces",
		Status:   report.StatusPass,
	}
	if len(namespaces) == 0 {
		res.Summary = "No add-on namespaces configured; skipping."
		return res
	}
	if kc == nil || kc.Clientset == nil {
		res.Status = report.StatusFail
		res.Summary = "Missing Kubernetes clientset."
		return res
	}

	nsSet := map[string]struct{}{}
	for _, ns := range namespaces {
		nsSet[ns] = struct{}{}
	}

	pods, err := kc.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Failed to list pods: %v", err)
		return res
	}

	var badPods []string
	for i := range pods.Items {
		p := &pods.Items[i]
		if _, ok := nsSet[p.Namespace]; !ok {
			continue
		}
		if podIsFailing(p) {
			badPods = append(badPods, fmt.Sprintf("%s/%s phase=%s", p.Namespace, p.Name, p.Status.Phase))
		}
	}

	// Webhook sanity: check for webhooks whose backing service is in one of these namespaces
	// and appears to have no endpoints.
	var badWebhooks []string
	mut, _ := kc.Clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	val, _ := kc.Clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})

	for i := range mut.Items {
		wc := &mut.Items[i]
		badWebhooks = append(badWebhooks, inspectWebhookConfig(ctx, kc, wc.Name, wc.Webhooks, nsSet)...)
	}
	for i := range val.Items {
		wc := &val.Items[i]
		badWebhooks = append(badWebhooks, inspectWebhookConfigValidating(ctx, kc, wc.Name, wc.Webhooks, nsSet)...)
	}

	sort.Strings(badPods)
	sort.Strings(badWebhooks)

	if len(badPods) > 0 {
		res.Status = report.WorstStatus(res.Status, report.StatusFail)
		res.Details = append(res.Details, fmt.Sprintf("Failing add-on pods: %d", len(badPods)))
		res.Details = append(res.Details, "Examples: "+strings.Join(truncate(badPods, 10), ", "))
	}
	if len(badWebhooks) > 0 {
		res.Status = report.WorstStatus(res.Status, report.StatusFail)
		res.Details = append(res.Details, fmt.Sprintf("Webhook service/endpoints issues: %d", len(badWebhooks)))
		res.Details = append(res.Details, "Examples: "+strings.Join(truncate(badWebhooks, 10), " | "))
	}

	if res.Status == report.StatusFail {
		res.Summary = "Critical add-on components are unhealthy."
		res.NextSteps = []string{
			"kubectl get pods -A",
			"kubectl get mutatingwebhookconfigurations,validatingwebhookconfigurations",
		}
		return res
	}
	res.Summary = "No failing add-on pods/webhooks detected."
	return res
}

func podIsFailing(p *corev1.Pod) bool {
	if p.Status.Phase == corev1.PodFailed {
		return true
	}
	if r := podBackoffReason(p); r != "" {
		return true
	}
	// Running but not Ready.
	if p.Status.Phase == corev1.PodRunning {
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status != corev1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func inspectWebhookConfig(ctx context.Context, kc *kube.Client, configName string, webhooks []admv1.MutatingWebhook, nsSet map[string]struct{}) []string {
	// This function handles MutatingWebhook; ValidatingWebhook has same structure for clientConfig.
	var out []string
	for i := range webhooks {
		wh := &webhooks[i]
		if wh.ClientConfig.Service == nil {
			continue
		}
		ns := wh.ClientConfig.Service.Namespace
		if _, ok := nsSet[ns]; !ok {
			continue
		}
		svcName := wh.ClientConfig.Service.Name
		svc, err := kc.Clientset.CoreV1().Services(ns).Get(ctx, svcName, metav1.GetOptions{})
		if err != nil || svc == nil {
			out = append(out, fmt.Sprintf("%s webhook=%s service=%s/%s missing", configName, wh.Name, ns, svcName))
			continue
		}
		// Use Endpoints for portability.
		ep, err := kc.Clientset.CoreV1().Endpoints(ns).Get(ctx, svcName, metav1.GetOptions{})
		if err != nil || ep == nil {
			out = append(out, fmt.Sprintf("%s webhook=%s endpoints=%s/%s missing", configName, wh.Name, ns, svcName))
			continue
		}
		addrs := 0
		for _, ss := range ep.Subsets {
			addrs += len(ss.Addresses)
		}
		if addrs == 0 {
			out = append(out, fmt.Sprintf("%s webhook=%s service=%s/%s has 0 endpoints", configName, wh.Name, ns, svcName))
		}
	}
	return out
}

// Overload for ValidatingWebhookConfiguration.
func inspectWebhookConfigValidating(ctx context.Context, kc *kube.Client, configName string, webhooks []admv1.ValidatingWebhook, nsSet map[string]struct{}) []string {
	var out []string
	for i := range webhooks {
		wh := &webhooks[i]
		if wh.ClientConfig.Service == nil {
			continue
		}
		ns := wh.ClientConfig.Service.Namespace
		if _, ok := nsSet[ns]; !ok {
			continue
		}
		svcName := wh.ClientConfig.Service.Name
		_, err := kc.Clientset.CoreV1().Services(ns).Get(ctx, svcName, metav1.GetOptions{})
		if err != nil {
			out = append(out, fmt.Sprintf("%s webhook=%s service=%s/%s missing", configName, wh.Name, ns, svcName))
			continue
		}
		ep, err := kc.Clientset.CoreV1().Endpoints(ns).Get(ctx, svcName, metav1.GetOptions{})
		if err != nil {
			out = append(out, fmt.Sprintf("%s webhook=%s endpoints=%s/%s missing", configName, wh.Name, ns, svcName))
			continue
		}
		addrs := 0
		for _, ss := range ep.Subsets {
			addrs += len(ss.Addresses)
		}
		if addrs == 0 {
			out = append(out, fmt.Sprintf("%s webhook=%s service=%s/%s has 0 endpoints", configName, wh.Name, ns, svcName))
		}
	}
	return out
}
