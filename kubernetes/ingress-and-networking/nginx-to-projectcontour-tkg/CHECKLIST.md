# Execution Checklist: NGINX to Contour (TKG)

## Manifest and Image Preparation
- [ ] `contour.yaml` downloaded from `release-1.31` URL.
- [ ] Upstream image references identified.
- [ ] Contour and Envoy images pulled.
- [ ] Contour and Envoy images tagged/pushed to Harbor.
- [ ] `contour.yaml` image references replaced with Harbor paths.

## Cluster Pre-Checks
- [ ] Target cluster context verified.
- [ ] Existing ingress resources backed up.
- [ ] Existing NGINX ingress classes/webhooks/CRDs inventoried.

## NGINX Full Removal (Before Contour)
- [ ] NGINX ingressclass removed.
- [ ] NGINX CRDs removed.
- [ ] NGINX validating/mutating webhooks removed.
- [ ] NGINX namespace removed.
- [ ] Post-removal verification (`crd`, `svc`, `api-resources`, `ingressclass`) done.

## Contour Deployment
- [ ] Contour manifest applied.
- [ ] Contour/Envoy pods healthy.
- [ ] Contour CRDs present.
- [ ] Contour ingress class present.

## Route Cutover
- [ ] HTTPProxy manifests created per hostname/service.
- [ ] HTTPProxy objects applied successfully.
- [ ] HTTPProxy status accepted/valid.
- [ ] External HTTPS checks pass for all migrated hostnames.

## Rollback Readiness
- [ ] Previous NGINX manifests available.
- [ ] Ingress backup file available and restorable.
