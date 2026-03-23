package checks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CheckWorkloadReadiness(ctx context.Context, kc *kube.Client, includeNS, excludeNS []string) report.CheckResult {
	res := report.CheckResult{
		ID:       "C",
		Category: "Workload readiness",
		Status:   report.StatusPass,
		NextSteps: []string{
			"kubectl get pods -A",
			"kubectl get deploy,statefulset,daemonset -A",
		},
	}
	if kc == nil || kc.Clientset == nil {
		res.Status = report.StatusFail
		res.Summary = "Missing Kubernetes clientset."
		return res
	}

	nsFilter := newNamespaceFilter(includeNS, excludeNS)

	pods, err := kc.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Failed to list pods: %v", err)
		return res
	}

	var crashLoop []string
	var imagePull []string
	var pendingOld []string

	now := time.Now()
	for i := range pods.Items {
		p := &pods.Items[i]
		if !nsFilter.Allows(p.Namespace) {
			continue
		}
		if reason := podBackoffReason(p); reason != "" {
			switch reason {
			case "CrashLoopBackOff":
				crashLoop = append(crashLoop, fmt.Sprintf("%s/%s", p.Namespace, p.Name))
			case "ImagePullBackOff", "ErrImagePull":
				imagePull = append(imagePull, fmt.Sprintf("%s/%s", p.Namespace, p.Name))
			}
		}
		if p.Status.Phase == corev1.PodPending && now.Sub(p.CreationTimestamp.Time) > 5*time.Minute {
			pendingOld = append(pendingOld, fmt.Sprintf("%s/%s age=%s", p.Namespace, p.Name, now.Sub(p.CreationTimestamp.Time).Round(time.Second)))
		}
	}

	deploys, err := kc.Clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Failed to list deployments: %v", err)
		return res
	}
	sts, err := kc.Clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Failed to list statefulsets: %v", err)
		return res
	}
	ds, err := kc.Clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Failed to list daemonsets: %v", err)
		return res
	}

	var controllers []string
	for i := range deploys.Items {
		d := &deploys.Items[i]
		if !nsFilter.Allows(d.Namespace) {
			continue
		}
		if d.Status.UnavailableReplicas > 0 {
			controllers = append(controllers, fmt.Sprintf("deploy %s/%s unavailable=%d", d.Namespace, d.Name, d.Status.UnavailableReplicas))
		}
	}
	for i := range sts.Items {
		s := &sts.Items[i]
		if !nsFilter.Allows(s.Namespace) {
			continue
		}
		want := int32(1)
		if s.Spec.Replicas != nil {
			want = *s.Spec.Replicas
		}
		unavail := want - s.Status.ReadyReplicas
		if unavail > 0 {
			controllers = append(controllers, fmt.Sprintf("sts %s/%s unavailable=%d", s.Namespace, s.Name, unavail))
		}
	}
	for i := range ds.Items {
		d := &ds.Items[i]
		if !nsFilter.Allows(d.Namespace) {
			continue
		}
		if d.Status.NumberUnavailable > 0 {
			controllers = append(controllers, fmt.Sprintf("ds %s/%s unavailable=%d", d.Namespace, d.Name, d.Status.NumberUnavailable))
		}
	}

	sort.Strings(crashLoop)
	sort.Strings(imagePull)
	sort.Strings(pendingOld)
	sort.Strings(controllers)

	if len(crashLoop) > 0 || len(imagePull) > 0 || len(pendingOld) > 0 || len(controllers) > 0 {
		res.Status = report.StatusFail
		res.Summary = "Detected workload readiness issues which can block safe rollouts."
	}

	if len(crashLoop) > 0 {
		res.Details = append(res.Details, fmt.Sprintf("CrashLoopBackOff: %d pod(s)", len(crashLoop)))
		res.Details = append(res.Details, "Examples: "+strings.Join(truncate(crashLoop, 10), ", "))
	}
	if len(imagePull) > 0 {
		res.Details = append(res.Details, fmt.Sprintf("ImagePullBackOff/ErrImagePull: %d pod(s)", len(imagePull)))
		res.Details = append(res.Details, "Examples: "+strings.Join(truncate(imagePull, 10), ", "))
	}
	if len(pendingOld) > 0 {
		res.Details = append(res.Details, fmt.Sprintf("Pending >5m: %d pod(s)", len(pendingOld)))
		res.Details = append(res.Details, "Examples: "+strings.Join(truncate(pendingOld, 10), ", "))
	}
	if len(controllers) > 0 {
		res.Details = append(res.Details, fmt.Sprintf("Controllers unavailable: %d object(s)", len(controllers)))
		res.Details = append(res.Details, "Examples: "+strings.Join(truncate(controllers, 10), ", "))
	}

	if res.Summary == "" {
		res.Summary = "No CrashLoopBackOff/ImagePullBackOff/Pending>5m pods; controllers are fully available."
	}
	return res
}

func podBackoffReason(p *corev1.Pod) string {
	// Check init + app containers for the first "interesting" backoff reason.
	for _, st := range append(p.Status.InitContainerStatuses, p.Status.ContainerStatuses...) {
		if st.State.Waiting == nil {
			continue
		}
		r := st.State.Waiting.Reason
		switch r {
		case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull":
			return r
		}
	}
	return ""
}

func truncate(ss []string, n int) []string {
	if len(ss) <= n {
		return ss
	}
	return ss[:n]
}

type namespaceFilter struct {
	include map[string]struct{}
	exclude map[string]struct{}
}

func newNamespaceFilter(include, exclude []string) namespaceFilter {
	f := namespaceFilter{
		include: map[string]struct{}{},
		exclude: map[string]struct{}{},
	}
	for _, ns := range include {
		f.include[ns] = struct{}{}
	}
	for _, ns := range exclude {
		f.exclude[ns] = struct{}{}
	}
	return f
}

func (f namespaceFilter) Allows(ns string) bool {
	if len(f.include) > 0 {
		_, ok := f.include[ns]
		return ok
	}
	if _, ok := f.exclude[ns]; ok {
		return false
	}
	return true
}
