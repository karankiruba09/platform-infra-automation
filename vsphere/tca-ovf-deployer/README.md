# TCA OVA Multi-Site Deployer

Concurrent, repeatable ovftool wrapper for deploying the Telco Cloud Automation OVA to many vCenters at once (10+ sites). All inputs are driven from a JSON config; passwords can be provided via environment variables to avoid storing secrets in git.

## Features
- Validates all required inputs up front (OVA path, ovftool path, networking, passwords, etc.).
- Runs deployments concurrently with a configurable worker pool and per-site timeouts.
- Per-site log folders with ovftool log + stdout/stderr, plus dry-run to review commands.
- Supports all properties from the manual ovftool command: IP/gateway/prefix, DNS/NTP, CLI/root/TCA user passwords, hostname, external FQDN, SSH enablement, Workflow Hub, appliance role, network mapping, disk/deployment options, hidden properties.
- Target selection: run all sites or a comma-separated subset (`-sites site1,site2`).

## Layout
- `main.go` — deployment CLI.
- `inputs/deploy.example.json` — starter config to copy and edit.
- `inputs/deploy.json` — your real config (gitignored).
- `output/logs/` — per-site deployment logs (gitignored).
- `Makefile` — build, test, run, dry-run, and clean targets.

## Build
```bash
make build          # compiles to bin/tca-ovf-deployer
make test           # runs go test ./...
make run            # build + run with inputs/deploy.json
make dry-run        # build + run with -dry-run flag
make clean          # remove bin/ and output/
```

## Quick start
1) Copy the example config and edit it:
   ```bash
   cd platform-infra-automation/vsphere/tca-ovf-deployer
   cp inputs/deploy.example.json inputs/deploy.json
   ```
   Update paths, site entries, and (optionally) inline passwords. Prefer env vars for secrets.

2) Export passwords (recommended):
   ```bash
   export VCENTER_PASSWORD='<your-vcenter-password>'
   export TCA_APPLIANCE_PASSWORD='<your-appliance-password>'
   ```

3) Run all sites (default worker pool size is auto-calculated; override with `-workers N`):
   ```bash
   go run . -config inputs/deploy.json
   ```

4) Run a subset:
   ```bash
   go run . -config inputs/deploy.json -sites site-a-az01,site-b-az01
   ```

5) Dry run (print commands with secrets masked, no execution):
   ```bash
   go run . -config inputs/deploy.json -sites site-a-az01 -dry-run
   ```

## Config reference (`inputs/deploy.example.json`)
- `common` block sets defaults; any field can be overridden per-site.
- Password sourcing order per field: site env var → site inline → common env var → common inline → error.
- Key fields
  - `ovfToolPath`: absolute path to `ovftool` binary.
  - `ovaPath`: path to the TCA OVA.
  - `username` / `vcenterPasswordEnv|vcenterPassword`: vCenter credentials.
  - `datastore`, `cluster`, `vmFolder`, `networkMappings`: placement and network mapping (e.g., `{ "VSMgmt": "Management VMs" }`).
  - `diskMode`, `deploymentOption`: match OVA options (e.g., `XLarge-ForTCAManagerwithWorkflowHubDeployments`).
  - `dnsList`, `ntpList`: arrays of IPs.
  - `mgrCliPassword*`, `mgrRootPassword*`, `tcaUserPassword*`: appliance credentials.
  - `hostname`, `externalAddress`, `ip`, `prefix`, `gateway`: per-site addressing.
  - `sshEnabled`, `serviceWFH`, `applianceRole`: appliance toggles.
  - `waitForIPSeconds`, `timeoutMinutes`: ovftool wait and process timeout.
  - `workerCount`: concurrency (default auto if 0).

## Logs
Logs land under `output/logs/<site-id>/`:
- `ovf_deployment.log` (ovftool log)
- `ovf_stdout.log`
- `ovf_stderr.log`

## Notes / Safety
- The tool masks secrets in dry-run output but ovftool still receives real passwords at runtime.
- Ensure DNS entries in the config are comma-separated after join; arrays are easier to edit safely.
- Hidden-property flags (`--X:enableHiddenProperties`, `--X:injectOvfEnv`, `--X:waitForIp`) are enabled by default.
- If gofmt/go run fails on this host (snap confinement), run the tool from a workstation with Go 1.21+.
