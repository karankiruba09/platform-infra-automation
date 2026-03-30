# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Purpose

Production infrastructure operations toolkit for VMware ecosystem (vSphere, Avi, NSX, TCA) and Kubernetes cluster lifecycle management. Each top-level directory is a domain, and each subdirectory is a self-contained tool with its own README.

## Repository Structure

- `kubernetes/` — K8s cluster lifecycle, ingress migrations, registry tooling, pre-upgrade checks
- `vsphere/` — ESXi dashboards (Flask), vCenter backup auditing, OVA deployment
- `networking/` — Avi load balancer and NSX automation, license utilization reporting
- `storage/` — vSAN utilities
- `platform-upgrades/` — Upgrade scripts (PowerShell for vSphere, Bash for Aria/vRealize)

## Languages and Tooling by Project Type

### Go projects (`kubernetes/tca-tkg-precheck`, `vsphere/tca-ovf-deployer`)

```bash
make build    # go build -o bin/<binary> ./cmd/<name>
make test     # go test ./...
make run      # go run ./cmd/<name>
make clean    # rm -rf bin
```

Requires Go 1.21+. Uses `client-go` for Kubernetes API access.

### Python/Flask projects (`vsphere/esxi-upgrade-dashboard`, `vsphere/esxi-version-dashboard`)

```bash
pip install -r requirements.txt
python3 collector.py   # background data collector (pyvmomi → vCenter API)
python3 api.py         # Flask web server
```

Uses PyVmomi for vSphere API access.

### Ansible projects (`vsphere/vc-config-backup-report`, `networking/nsx-config-backup-manager`, `networking/avi-virtualservice-templater-ansible`)

```bash
pip install -r requirements.txt
ansible-playbook <playbook>.yml -i inventory.yml --ask-vault-pass
```

Credentials are Ansible Vault-encrypted. Inventory files define target hosts/vCenters.

### Bash/PowerShell (`platform-upgrades/`)

Run directly; no build step.

## Architecture Patterns

**Collector + API split** (ESXi dashboards): A `collector.py` process polls vCenter on an interval and writes JSON state to disk; `api.py` serves that state via Flask. They share a file path — not a database.

**Ansible + Jinja2 templating** (networking tools): Playbooks drive all API calls to Avi/NSX. Variables are split between `group_vars/` (defaults) and encrypted vault files. The Avi VS templater compares two existing VSes to extract a common Jinja2 template, then renders new instances with only the delta values.

**Go CLI tools**: Single-binary CLIs. `tca-tkg-precheck` performs pre-upgrade health checks against K8s/TCA APIs and emits a structured report. `tca-ovf-deployer` deploys OVAs concurrently to multiple vCenters using goroutines.

**Secrets**: Never committed. Credentials go in Ansible Vault-encrypted vars files or are passed as CLI flags at runtime. `.gitignore` is configured to exclude `*.yml` config files in several project directories.
