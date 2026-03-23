package checks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func CheckTKGManagementAwareness(ctx context.Context, kc *kube.Client) report.CheckResult {
	res := report.CheckResult{
		ID:       "D",
		Category: "TKG awareness",
		Status:   report.StatusPass,
		NextSteps: []string{
			"kubectl get clusters.cluster.x-k8s.io -A",
			"kubectl get tanzukubernetesreleases -A",
		},
	}
	if kc == nil || kc.Dynamic == nil || kc.Discovery == nil {
		res.Status = report.StatusFail
		res.Summary = "Missing Kubernetes dynamic/discovery client."
		return res
	}

	capiGV := "cluster.x-k8s.io/v1beta1"
	if !hasAPIResource(ctx, kc, capiGV, "clusters") {
		res.Summary = "Cluster API not detected; skipping TKG management cluster checks."
		return res
	}

	// Cluster API detected; treat as a management cluster for precheck purposes.
	res.Details = append(res.Details, "Cluster API detected (likely a TKG management cluster).")

	clusterGVR := schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "clusters"}
	cl, err := kc.Dynamic.Resource(clusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusWarn
		res.Summary = fmt.Sprintf("Failed to list Cluster API Clusters: %v", err)
		return res
	}

	tkrVersions := listTKRVersions(ctx, kc)

	var lines []string
	anyNotReady := false
	for i := range cl.Items {
		c := &cl.Items[i]
		ns := c.GetNamespace()
		name := c.GetName()
		ver := clusterKubernetesVersion(ctx, kc, c)
		ready := capiReadyCondition(c)
		if ready == "False" {
			anyNotReady = true
		}
		eligible := "-"
		if ver != "" && len(tkrVersions) > 0 {
			if next := nextPatch(ver, tkrVersions); next != "" && next != ver {
				eligible = next
			} else {
				eligible = "(none found)"
			}
		}
		lines = append(lines, fmt.Sprintf("%s/%s version=%s ready=%s eligibleUpgrade=%s", ns, name, orDash(ver), ready, eligible))
	}
	sort.Strings(lines)
	if len(lines) > 0 {
		res.Details = append(res.Details, fmt.Sprintf("Clusters discovered: %d", len(lines)))
		res.Details = append(res.Details, "Examples: "+strings.Join(truncate(lines, 10), " | "))
	} else {
		res.Details = append(res.Details, "No Cluster API Clusters found.")
	}

	if anyNotReady {
		res.Status = report.StatusWarn
		res.Summary = "One or more Cluster API Clusters are not Ready."
		return res
	}
	res.Summary = "Cluster API Clusters enumerated."
	return res
}

func hasAPIResource(ctx context.Context, kc *kube.Client, groupVersion, resource string) bool {
	rl, err := kc.Discovery.ServerResourcesForGroupVersion(groupVersion)
	if err != nil || rl == nil {
		return false
	}
	for _, r := range rl.APIResources {
		if r.Name == resource {
			return true
		}
	}
	return false
}

func capiReadyCondition(obj *unstructured.Unstructured) string {
	conds, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return "Unknown"
	}
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		if t != "Ready" {
			continue
		}
		s, _ := m["status"].(string)
		if s == "" {
			return "Unknown"
		}
		return s
	}
	return "Unknown"
}

func clusterKubernetesVersion(ctx context.Context, kc *kube.Client, c *unstructured.Unstructured) string {
	if v, found, _ := unstructured.NestedString(c.Object, "spec", "topology", "version"); found {
		return strings.TrimSpace(v)
	}
	ref, found, _ := unstructured.NestedMap(c.Object, "spec", "controlPlaneRef")
	if !found {
		return ""
	}
	apiVersion, _ := ref["apiVersion"].(string)
	kind, _ := ref["kind"].(string)
	name, _ := ref["name"].(string)
	if apiVersion == "" || kind == "" || name == "" {
		return ""
	}

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return ""
	}
	// Common case: KubeadmControlPlane has spec.version.
	gvr := schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: strings.ToLower(kind) + "s"}
	cp, err := kc.Dynamic.Resource(gvr).Namespace(c.GetNamespace()).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	if v, found, _ := unstructured.NestedString(cp.Object, "spec", "version"); found {
		return strings.TrimSpace(v)
	}
	return ""
}

func listTKRVersions(ctx context.Context, kc *kube.Client) []string {
	// Best-effort: TKR resources are usually present on a TKG management cluster.
	// Group/version varies by release, so search by kind.
	refs, err := findGVRsByKind(ctx, kc, "TanzuKubernetesRelease")
	if err != nil || len(refs) == 0 {
		return nil
	}
	gvr := refs[0]
	list, err := kc.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	var vers []string
	for i := range list.Items {
		it := &list.Items[i]
		if v, found, _ := unstructured.NestedString(it.Object, "spec", "version"); found && strings.TrimSpace(v) != "" {
			vers = append(vers, strings.TrimSpace(v))
		}
	}
	sort.Strings(vers)
	return vers
}

func nextPatch(current string, available []string) string {
	cur := parseSemver(current)
	if cur.Major == 0 {
		return ""
	}
	best := semver{}
	bestStr := ""
	for _, a := range available {
		v := parseSemver(a)
		if v.Major != cur.Major || v.Minor != cur.Minor {
			continue
		}
		if v.Patch <= cur.Patch {
			continue
		}
		if bestStr == "" || v.Patch < best.Patch {
			best = v
			bestStr = strings.TrimSpace(a)
		}
	}
	return bestStr
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
