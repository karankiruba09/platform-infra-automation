# vCenter Backup Reporter

Ansible automation that audits backup configuration across vCenter appliances and generates a CSV report with schedules and latest backup job status.

## Quick Start

```bash
# 1) Install dependencies
pip install -r requirements.txt

# 2) Edit vault.yml with credentials
vim vault.yml

# 3) Encrypt the vault
ansible-vault encrypt vault.yml

# 4) Add vCenter hosts in inventory.yml
vim inventory.yml

# 5) Run the report
ansible-playbook vcenter_backup_report.yml -i inventory.yml --ask-vault-pass
```

Output files:
- `./reports/vcenter_backup_data.json`
- `./reports/vcenter_backup_report.csv`

## Project Files

```text
vc-config-backup-report/
├── vcenter_backup_report.yml      # Main playbook
├── generate_backup_report_csv.py  # CSV generator
├── inventory.yml                  # vCenter host inventory
├── vault.yml                      # Encrypted credentials
├── requirements.txt               # Python/Ansible dependencies
└── reports/                       # Generated artifacts
```

## Configuration

### Credentials (`vault.yml`)

```yaml
vcenter_user: "administrator@vsphere.local"
vcenter_password: "YourPasswordHere"
```

Encrypt/edit with:

```bash
ansible-vault encrypt vault.yml
ansible-vault edit vault.yml
```

### Inventory (`inventory.yml`)

```yaml
all:
  children:
    vcenters:
      hosts:
        vcenter01.prod.local:
        vcenter02.prod.local:
```

Optional:

```yaml
all:
  vars:
    vcenter_validate_certs: false
```

## Usage

Basic run:

```bash
ansible-playbook vcenter_backup_report.yml -i inventory.yml --ask-vault-pass
```

With vault password file:

```bash
ansible-playbook vcenter_backup_report.yml -i inventory.yml --vault-password-file .vault_pass
```

Custom output path:

```bash
ansible-playbook vcenter_backup_report.yml -i inventory.yml --ask-vault-pass -e "report_output_dir=/opt/reports"
```

Single vCenter:

```bash
ansible-playbook vcenter_backup_report.yml -i inventory.yml --ask-vault-pass --limit vcenter01.prod.local
```

## CSV Columns

The report contains:

- `vCenter`, `Version`, `Build`, `Timezone`
- `Schedules`, `Enabled`
- `Backup Location`, `Type`
- `Recurrence`, `Retention`
- `Last Job`, `Status`
- `Start`, `End`, `Duration`, `Size (MB)`

The latest job is selected from `/api/appliance/recovery/backup/job/details`, including environments where the API returns a map of job IDs.

## API Endpoints Used

- `POST /api/session`
- `GET /api/appliance/recovery/backup/schedules`
- `GET /api/appliance/recovery/backup/job/details`
- `GET /api/appliance/system/version`
- `GET /api/appliance/system/time`
- `DELETE /api/session`

## Troubleshooting

### `UNKNOWN`/`N/A` in job fields

- Ensure the account has `Appliance.Backup.Configuration` read privilege.
- Re-run and check `reports/vcenter_backup_data.json` contains `job_details`.

### No schedules

- Backup has not been configured on that vCenter.

### Auth errors

- Verify username/password and SSO domain (for example `administrator@vsphere.local`).

## Requirements

- Python 3.8+
- Ansible 2.12+
- vCenter 6.7+
