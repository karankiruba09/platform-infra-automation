package checks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CheckNodesAndVersionSkew(ctx context.Context, kc *kube.Client) report.CheckResult {
	res := report.CheckResult{
		ID:       "B",
		Category: "Node and version skew",
		Status:   report.StatusPass,
		NextSteps: []string{
			"kubectl get nodes -o wide",
			"kubectl describe node <node-name>",
		},
	}
	if kc == nil || kc.Clientset == nil {
		res.Status = report.StatusFail
		res.Summary = "Missing Kubernetes clientset."
		return res
	}

	nodes, err := kc.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Failed to list nodes: %v", err)
		return res
	}

	var notReady []string
	var cordoned []string
	var pressure []string

	kubeletVersions := map[string]int{}
	for i := range nodes.Items {
		n := &nodes.Items[i]
		if !nodeReady(n) {
			notReady = append(notReady, n.Name)
		}
		if n.Spec.Unschedulable {
			cordoned = append(cordoned, n.Name)
		}
		for _, t := range []corev1.NodeConditionType{
			corev1.NodeMemoryPressure,
			corev1.NodeDiskPressure,
			corev1.NodePIDPressure,
		} {
			if nodeConditionTrue(n, t) {
				pressure = append(pressure, fmt.Sprintf("%s(%s)", n.Name, t))
			}
		}
		kv := strings.TrimSpace(n.Status.NodeInfo.KubeletVersion)
		if kv != "" {
			kubeletVersions[kv]++
		}
	}

	cpVersion := ""
	if sv, err := kc.Clientset.Discovery().ServerVersion(); err == nil && sv != nil {
		cpVersion = sv.GitVersion
	}

	if len(notReady) > 0 {
		sort.Strings(notReady)
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("%d node(s) NotReady.", len(notReady))
		res.Details = append(res.Details, fmt.Sprintf("NotReady: %s", strings.Join(notReady, ", ")))
	}
	if len(pressure) > 0 {
		sort.Strings(pressure)
		res.Status = report.WorstStatus(res.Status, report.StatusFail)
		if res.Summary == "" {
			res.Summary = "Node pressure condition(s) detected."
		}
		res.Details = append(res.Details, fmt.Sprintf("Pressure: %s", strings.Join(pressure, ", ")))
	}
	if len(cordoned) > 0 {
		sort.Strings(cordoned)
		res.Status = report.WorstStatus(res.Status, report.StatusWarn)
		if res.Summary == "" || res.Status == report.StatusWarn {
			res.Summary = fmt.Sprintf("%d node(s) cordoned/unschedulable.", len(cordoned))
		}
		res.Details = append(res.Details, fmt.Sprintf("Cordoned: %s", strings.Join(cordoned, ", ")))
	}

	if cpVersion != "" {
		res.Details = append(res.Details, fmt.Sprintf("Control plane: %s", cpVersion))
	}
	res.Details = append(res.Details, fmt.Sprintf("Kubelet versions: %s", formatVersionCounts(kubeletVersions)))

	skewWarn := describeSkew(cpVersion, kubeletVersions)
	if skewWarn != "" {
		res.Status = report.WorstStatus(res.Status, report.StatusWarn)
		res.Details = append(res.Details, skewWarn)
	}

	if res.Summary == "" {
		res.Summary = "All nodes Ready; no pressure detected."
	}
	return res
}

func nodeReady(n *corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func nodeConditionTrue(n *corev1.Node, t corev1.NodeConditionType) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == t {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func formatVersionCounts(m map[string]int) string {
	if len(m) == 0 {
		return "(none)"
	}
	type kv struct {
		v string
		n int
	}
	var pairs []kv
	for v, n := range m {
		pairs = append(pairs, kv{v: v, n: n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].v < pairs[j].v
	})
	var out []string
	for _, p := range pairs {
		out = append(out, fmt.Sprintf("%s=%d", p.v, p.n))
	}
	return strings.Join(out, ", ")
}

func describeSkew(controlPlane string, kubeletVersions map[string]int) string {
	cp := parseSemver(controlPlane)
	if cp.Major == 0 {
		return ""
	}
	maxSkew := 0
	for kv := range kubeletVersions {
		v := parseSemver(kv)
		if v.Major != cp.Major || v.Minor == 0 {
			continue
		}
		skew := cp.Minor - v.Minor
		if skew < 0 {
			skew = -skew
		}
		if skew > maxSkew {
			maxSkew = skew
		}
	}
	if maxSkew >= 2 {
		return fmt.Sprintf("Version skew: kubelet minor version differs from control plane by up to %d (review upgrade ordering).", maxSkew)
	}
	return ""
}

type semver struct {
	Major int
	Minor int
	Patch int
}

func parseSemver(s string) semver {
	// Accept v1.29.3 or 1.29.3.
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return semver{}
	}
	var v semver
	_, _ = fmt.Sscanf(parts[0], "%d", &v.Major)
	_, _ = fmt.Sscanf(parts[1], "%d", &v.Minor)
	if len(parts) >= 3 {
		_, _ = fmt.Sscanf(parts[2], "%d", &v.Patch)
	}
	return v
}
