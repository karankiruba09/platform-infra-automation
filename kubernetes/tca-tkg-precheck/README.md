# tca-tkg-precheck

Single-run pre-upgrade health report CLI tailored for VMware Telco Cloud Automation (TCA) with Tanzu Kubernetes Grid (TKG) clusters.

## How This Differs For TCA And TKG

- TKG: Detects Cluster API (CAPI) on the management cluster and enumerates `Cluster` objects with Kubernetes versions (and a best-effort "eligible next patch" from `TanzuKubernetesRelease` versions if present).
- TCA: Reads `TcaKubernetesCluster` custom resources (when present) and surfaces the `telco.vmware.com/airgap-ca-cert` annotation, including decoded certificate subject and expiry. With `--check-airgap`, it uses that decoded CA to validate TLS and reachability to a user-provided registry endpoint.

## Run With An Existing Pinniped Kubeconfig

This tool uses standard `client-go` kubeconfig loading rules (no embedded credentials). It assumes your kubeconfig is already refreshed and your context is selected.

Build and run:

```bash
cd kubernetes/tca-tkg-precheck && make build && ./bin/precheck --output-md report.md --timeout-seconds 60
```

Target a specific kubeconfig context:

```bash
cd kubernetes/tca-tkg-precheck && ./bin/precheck --kubecontext <context-name> --output-md report.md
```

## Refresh A Pinniped Kubeconfig

Using `pinniped` CLI (placeholders for issuer URL and client ID):

```bash
pinniped get kubeconfig --oidc-issuer <https://issuer.example.com> --oidc-client-id <client-id> > <path-to-output-kubeconfig>
```

Using `kubectl oidc-login` (plugin) (placeholders for issuer URL and client ID):

```bash
kubectl oidc-login setup --oidc-issuer-url <https://issuer.example.com> --oidc-client-id <client-id> --kubeconfig <path-to-kubeconfig>
```

## Example With Air-Gap Check

```bash
cd kubernetes/tca-tkg-precheck && make build && ./bin/precheck --output-md report.md --check-airgap --airgap-endpoint https://registry.example.com/v2/
```

