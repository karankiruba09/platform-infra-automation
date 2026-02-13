# ESXi Version Dashboard (Multi-vCenter)

Fast, refresh-on-demand dashboard to track ESXi versions across many vCenters (for example 54). It collects host version/build via `govc` in parallel, then serves a small web UI.

## What You Get

- Holistic ESXi counts by major version:
  - `6.x`, `7.x`, `8.x`, `unknown/other`
- 8.x build breakdown (version + build numbers)
- Per-vCenter breakdown + overall totals
- Detailed host inventory export (`CSV`)
- Click **Refresh** in the dashboard: collector runs and updates in a few seconds (parallelized + per-vCenter timeout)

## Prereqs

- `govc` (tested with `govc 0.52.0`)
- `jq`
- `python3` (optional: render the HTML report + run the local dashboard server; no Node/JavaScript required)

## Configure

1) Copy `config/vcenters.txt.example` to `config/vcenters.txt` (the actual file is gitignored to keep secrets local), then edit it:

```text
# One per line:
#   name|vcenter-fqdn
# or:
#   vcenter-fqdn
prod-vc01|vcenter01.example.com
prod-vc02|vcenter02.example.com
```

2) Create `.env` (copy from `.env.example`) and set credentials:

```bash
cp .env.example .env
```

## Run

### 1) Collect Once (writes JSON + CSV)

```bash
./scripts/collect.sh
```

Artifacts:
- `public/esxi_versions.json`
- `public/esxi_hosts.csv`
- `public/report.html`

### 2) View Report (no server)

Open:
- `public/report.html`

### 3) Optional: Start Dashboard (local server)

```bash
python3 scripts/server.py
```

Open:
- `http://localhost:8081`

Use **Refresh** to re-collect live.

## Troubleshooting (No Data / All Zeros)

1) Run the collector and inspect the report:

```bash
./scripts/collect.sh
jq '.totals, (.rows[] | {vcenter, status, error})' public/esxi_versions.json
```

2) If you see `proxyconnect` or similar errors, keep `VC_UNSET_PROXY=true` in `.env` (recommended for internal vCenters).

3) Debug raw output (keeps temp dir and prints its path):

```bash
./scripts/collect.sh --debug-keep-tmp
```

## Tunables

Set in `.env`:

- `VC_PARALLEL` (default `20`) max concurrent vCenter collections
- `VC_TIMEOUT_SECONDS` (default `12`) per-vCenter timeout
- `VC_INSECURE` (default `true`) set `false` if you have proper TLS trust
- `VC_UNSET_PROXY` (default `true`) unset `HTTP(S)_PROXY`/`ALL_PROXY` for `govc` calls (recommended for internal vCenters)

## Notes

- The collector uses `govc object.collect` for a single bulk property query per vCenter (fast).
- `GOVC_PERSIST_SESSION=true` is used to speed up subsequent refreshes.
