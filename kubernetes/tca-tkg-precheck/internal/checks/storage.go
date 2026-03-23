package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/kube"
	"github.com/kirubakaran-kandhasa/platform-infra-automation/kubernetes/tca-tkg-precheck/internal/report"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func CheckStorageSanity(ctx context.Context, kc *kube.Client, enabled bool, storageClasses []string) report.CheckResult {
	res := report.CheckResult{
		ID:       "H",
		Category: "Storage sanity (optional)",
		Status:   report.StatusPass,
	}
	if !enabled {
		res.Summary = "Skipped (enable with --check-storage)."
		return res
	}
	if kc == nil || kc.Clientset == nil {
		res.Status = report.StatusFail
		res.Summary = "Missing Kubernetes clientset."
		return res
	}

	scs, err := kc.Clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = report.StatusFail
		res.Summary = fmt.Sprintf("Failed to list StorageClasses: %v", err)
		return res
	}

	toTest := storageClasses
	if len(toTest) == 0 {
		if def := defaultStorageClassName(scs.Items); def != "" {
			toTest = []string{def}
		}
	}
	toTest = report.UniqueSorted(toTest)
	if len(toTest) == 0 {
		res.Status = report.StatusFail
		res.Summary = "No StorageClass found to test (and none provided via --storageclasses)."
		return res
	}

	scByName := map[string]storagev1.StorageClass{}
	for i := range scs.Items {
		scByName[scs.Items[i].Name] = scs.Items[i]
	}

	var tested []string
	for _, sc := range toTest {
		scObj, ok := scByName[sc]
		if !ok {
			res.Status = report.StatusFail
			tested = append(tested, fmt.Sprintf("sc=%s missing", sc))
			continue
		}
		st, detail := testPVC(ctx, kc, scObj)
		res.Status = report.WorstStatus(res.Status, st)
		tested = append(tested, detail)
	}
	switch res.Status {
	case report.StatusPass:
		res.Summary = "PVC create/bind/delete succeeded."
	case report.StatusWarn:
		res.Summary = "PVC test completed with warnings (often expected with WaitForFirstConsumer StorageClasses)."
	case report.StatusFail:
		res.Summary = "PVC create/bind/delete test failed."
		res.NextSteps = []string{
			"kubectl get sc",
			"kubectl get pvc -A",
			"kubectl describe pvc -n default <pvc-name>",
		}
	}
	res.Details = append(res.Details, tested...)
	return res
}

func defaultStorageClassName(scs []storagev1.StorageClass) string {
	for i := range scs {
		sc := &scs[i]
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			return sc.Name
		}
	}
	return ""
}

func testPVC(ctx context.Context, kc *kube.Client, sc storagev1.StorageClass) (report.Status, string) {
	name := fmt.Sprintf("precheck-%d", time.Now().UnixNano())
	ns := "default"
	one := corev1.ResourceList{
		corev1.ResourceStorage: mustParseQuantity("1Mi"),
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: one,
			},
			StorageClassName: &sc.Name,
		},
	}

	// Always attempt cleanup.
	defer func() {
		_ = kc.Clientset.CoreV1().PersistentVolumeClaims(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	}()

	if _, err := kc.Clientset.CoreV1().PersistentVolumeClaims(ns).Create(ctx, pvc, metav1.CreateOptions{}); err != nil {
		return report.StatusFail, fmt.Sprintf("sc=%s pvc=%s/%s create error: %v", sc.Name, ns, name, err)
	}

	err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		got, err := kc.Clientset.CoreV1().PersistentVolumeClaims(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return got.Status.Phase == corev1.ClaimBound, nil
	})
	if err != nil {
		if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			return report.StatusWarn, fmt.Sprintf("sc=%s pvc=%s/%s not bound within 30s (volumeBindingMode=WaitForFirstConsumer; expected until a pod consumes it)", sc.Name, ns, name)
		}
		return report.StatusFail, fmt.Sprintf("sc=%s pvc=%s/%s bind timeout/error: %v", sc.Name, ns, name, err)
	}

	// Delete and confirm it's gone.
	if err := kc.Clientset.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return report.StatusFail, fmt.Sprintf("sc=%s pvc=%s/%s delete error: %v", sc.Name, ns, name, err)
	}
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := kc.Clientset.CoreV1().PersistentVolumeClaims(ns).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return report.StatusFail, fmt.Sprintf("sc=%s pvc=%s/%s delete timeout/error: %v", sc.Name, ns, name, err)
	}

	return report.StatusPass, fmt.Sprintf("sc=%s pvc=%s/%s ok", sc.Name, ns, name)
}

func mustParseQuantity(s string) resource.Quantity {
	return resource.MustParse(s)
}
