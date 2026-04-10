# vROps VM OS EOL Report

Connects to VMware Aria Operations (vROps) API, collects guest OS data for all VMs, and generates an OS end-of-life (EOL) summary report categorized by technology group.

## What You Get

- **Summary report** — VM counts by technology category (Linux, Ubuntu/Debian, Photon-OS, FreeBSD, Vendor-OVA, Windows) with Supported vs EOL breakdown and EOL percentage
- **Detail report** — Per-VM listing with guest OS, category, and EOL status
- **OS mapping reference** — All unique guest OS strings seen, with their EOL classification
- Output in CSV and JSON formats

### Sample Output

```
Technology      Footprint    Supported    OS-EOL    EOL%
Linux               2734         2534       200   7.32%
Ubuntu/Debian        530          528         2   0.38%
Photon-OS           1588         1588         0   0.00%
FreeBSD               14           13         1   7.14%
Vendor-OVA            67           67         0   0.00%
Windows              307          307         0   0.00%
Total               5240         5037       203   3.87%
```

## Prerequisites

- Python 3.8+ (for Ansible runtime)
- Ansible Core 2.12+
- Network access to vROps API (port 443)
- vROps user account with read access to VM resources

## Quick Start

```bash
# 1. Install Ansible
pip install -r requirements.txt

# 2. Copy and edit inventory
cp inputs/inventory.example.yml inputs/inventory.yml

# 3. Create and encrypt vault
cp inputs/vault.yml.example inputs/vault.yml
ansible-vault encrypt inputs/vault.yml

# 4. Update inputs/inventory.yml with your vROps hostnames
# 5. Update inputs/vault.yml with your vROps credentials

# 6. Run the playbook
ansible-playbook vrops_os_eol_report.yml --ask-vault-pass
```

## Configuration

### `inputs/inventory.yml`

```yaml
all:
  children:
    vrops:
      hosts:
        vrops01.example.com:
        vrops02.example.com:
  vars:
    vrops_validate_certs: false
    vrops_auth_source: local       # 'local' or your vIDM auth source name
    ansible_connection: local
    ansible_python_interpreter: "{{ ansible_playbook_python }}"
```

### `inputs/vault.yml`

```yaml
# Encrypted with ansible-vault
vrops_user: admin
vrops_password: changeme
```

### `eol_map.yml`

The EOL reference data lives in `eol_map.yml` at the project root. It maps vROps guest OS names to:
- **status**: `Supported` or `EOL`
- **category**: `Linux`, `Ubuntu/Debian`, `Photon-OS`, `FreeBSD`, `Vendor-OVA`, or `Windows`

Update this file when new OS versions are released or existing ones reach end-of-life. Unrecognized OS strings are auto-classified using keyword fallback rules defined in the same file.

## Output Files

All reports are written to the `output/` directory:

| File | Description |
|------|-------------|
| `os_eol_summary.csv` | Summary by technology category |
| `os_eol_detail.csv` | Per-VM guest OS and EOL status |
| `os_eol_mapping.csv` | All unique OS strings with EOL classification |
| `os_eol_report.json` | JSON report with metadata |

## File Structure

```
vrops-vm-os-eol-report/
├── README.md
├── requirements.txt
├── ansible.cfg
├── vrops_os_eol_report.yml     # Main playbook
├── eol_map.yml                 # OS EOL reference data (committed)
├── inputs/
│   ├── inventory.example.yml   # Inventory template
│   └── vault.yml.example       # Credentials template
├── output/
│   └── .gitkeep
└── templates/
    ├── os_eol_summary.csv.j2   # Summary report template
    ├── os_eol_detail.csv.j2    # Detail report template
    └── os_eol_mapping.csv.j2   # OS mapping template
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| `Connection refused` | Verify vROps hostname and network access to port 443 |
| `401 Unauthorized` | Check vrops_user/vrops_password in vault.yml |
| `SSL certificate error` | Set `vrops_validate_certs: false` in inventory |
| Missing VMs in report | Verify the vROps user has read access to VM resources |
| Unknown OS in output | Add the OS string to `eol_map.yml` |

## Notes

- SSL certificate verification is disabled by default. For production, set `vrops_validate_certs: true` and ensure valid certificates.
- Credentials are encrypted with Ansible Vault. Never commit unencrypted vault files.
- The playbook uses bulk property queries to minimize API calls to vROps.
- Multiple vROps instances can be listed in inventory; results are combined into a single report.
