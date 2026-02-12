# NGINX Ingress to Project Contour Migration (TKG)

## 1. Download the upstream Contour manifest
```bash
cd /tmp
curl -fsSLo contour.yaml https://raw.githubusercontent.com/projectcontour/contour/release-1.31/examples/render/contour.yaml
```

Validate the file and list referenced images:
```bash
test -s contour.yaml
grep -n 'image:' contour.yaml
```

For `release-1.31`, the expected image references are:
- `ghcr.io/projectcontour/contour:v1.31.3`
- `docker.io/envoyproxy/envoy:v1.34.12`

## 2. Mirror images to Harbor
You can run this project script to automate steps 1-3:
```bash
cd platform-infra-automation/kubernetes/ingress-and-networking/nginx-to-projectcontour-tkg
HARBOR_REGISTRY=<harbor-fqdn> \
HARBOR_PROJECT=<harbor-project> \
./scripts/prepare-contour-harbor.sh
```

Or run manually:
```bash
HARBOR_REGISTRY=<harbor-fqdn>
HARBOR_PROJECT=<harbor-project>
CONTOUR_TAG=v1.31.3
ENVOY_TAG=v1.34.12

SRC_CONTOUR=ghcr.io/projectcontour/contour:${CONTOUR_TAG}
SRC_ENVOY=docker.io/envoyproxy/envoy:${ENVOY_TAG}
DST_CONTOUR=${HARBOR_REGISTRY}/${HARBOR_PROJECT}/contour:${CONTOUR_TAG}
DST_ENVOY=${HARBOR_REGISTRY}/${HARBOR_PROJECT}/envoy:${ENVOY_TAG}

docker pull "${SRC_CONTOUR}"
docker pull "${SRC_ENVOY}"
docker tag "${SRC_CONTOUR}" "${DST_CONTOUR}"
docker tag "${SRC_ENVOY}" "${DST_ENVOY}"
docker push "${DST_CONTOUR}"
docker push "${DST_ENVOY}"

docker manifest inspect "${DST_CONTOUR}" >/dev/null
docker manifest inspect "${DST_ENVOY}" >/dev/null
```

## 3. Update `contour.yaml` to use Harbor images
```bash
sed -i "s|ghcr.io/projectcontour/contour:${CONTOUR_TAG}|${DST_CONTOUR}|g" contour.yaml
sed -i "s|docker.io/envoyproxy/envoy:${ENVOY_TAG}|${DST_ENVOY}|g" contour.yaml
grep -n 'image:' contour.yaml
```

## 4. Pre-check and backup on target cluster
```bash
kubectl config use-context <target-tkg-context>
kubectl config current-context
kubectl get ingress -A
kubectl get ingressclass
kubectl get ingress -A -o yaml > ingress-backup-$(date +%Y%m%d-%H%M%S).yaml
kubectl get crd | grep -i nginx || true
kubectl get validatingwebhookconfigurations.admissionregistration.k8s.io | grep -i nginx || true
kubectl get mutatingwebhookconfigurations.admissionregistration.k8s.io | grep -i nginx || true
kubectl get svc -A | grep -i nginx || true
```

## 5. Remove NGINX completely (before Contour install)
```bash
kubectl delete ingressclass nginx || true
kubectl delete crd \
  dnsendpoints.externaldns.nginx.org \
  globalconfigurations.k8s.nginx.org \
  policies.k8s.nginx.org \
  transportservers.k8s.nginx.org \
  virtualserverroutes.k8s.nginx.org \
  virtualservers.k8s.nginx.org || true
kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io ingress-nginx-admission || true
kubectl delete mutatingwebhookconfigurations.admissionregistration.k8s.io ingress-nginx-admission || true
kubectl delete ns nginx-ingress || true
```

Verify NGINX cleanup:
```bash
kubectl api-resources | grep -i nginx || true
kubectl get crd | grep -i nginx || true
kubectl get svc -A | grep -i nginx || true
kubectl get ingressclass
```

## 6. Install Contour using updated manifest
```bash
kubectl apply -f contour.yaml
kubectl get pods -n projectcontour
kubectl get crd | grep -i projectcontour
kubectl get ingressclass
```

Readiness gates:
- `projectcontour` pods are `Running` and `Ready`
- `httpproxies.projectcontour.io` exists
- Contour ingress class exists

## 7. Apply HTTPProxy resources
Use sanitized structure equivalent to your two-proxy YAML:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: <proxy-app-1>
  namespace: <app-namespace>
spec:
  virtualhost:
    fqdn: <app1.example.internal>
    tls:
      secretName: <tls-secret-app-1>
  routes:
    - conditions:
        - prefix: /
      services:
        - name: <service-app-1>
          port: 8080
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: <proxy-app-2>
  namespace: <app-namespace>
spec:
  virtualhost:
    fqdn: <app2.example.internal>
    tls:
      secretName: <tls-secret-app-2>
  routes:
    - conditions:
        - prefix: /
      services:
        - name: <service-app-2>
          port: 8080
```

```bash
kubectl apply -f <httpproxy-file>.yaml
kubectl get httpproxy -n <app-namespace>
kubectl describe httpproxy -n <app-namespace> <proxy-app-1>
kubectl describe httpproxy -n <app-namespace> <proxy-app-2>
```

## 8. Validate traffic
```bash
kubectl get pods -n projectcontour
kubectl get httpproxy -A
curl -Ik https://<app1.example.internal>
curl -Ik https://<app2.example.internal>
```

## 9. Rollback if needed
```bash
kubectl apply -f <nginx-controller-manifests>.yaml
kubectl apply -f ingress-backup-<timestamp>.yaml
kubectl delete -f <httpproxy-file>.yaml
```
