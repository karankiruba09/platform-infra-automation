# NSX Manager Backup Automation

Single Ansible project with two functions:
1. `report` mode: collect current NSX backup configuration and latest backup status from all NSX managers.
2. `configure` mode: apply backup configuration from an input policy file.

## Files

```text
nsx-config-backup-manager/
├── nsx_backup_manager.yml            # Single playbook (report + configure)
├── inventory.yml                     # NSX manager list
├── backup_policies.yml               # Input policy for configure mode
├── vault.yml.example                 # Credentials template
├── requirements.txt
├── templates/
│   └── nsx_backup_report.csv.j2
└── reports/                          # Generated report artifacts
```

## API Endpoints Used

- `GET /api/v1/cluster/backups/config`
- `PUT /api/v1/cluster/backups/config`
- `GET /api/v1/cluster/backups/history`
- `GET /api/v1/cluster/backups/status`
- `GET /api/v1/node/version`

## Quick Start

1. Install dependencies:

```bash
pip install -r requirements.txt
```

2. Prepare credentials:

```bash
cp vault.yml.example vault.yml
vim vault.yml
ansible-vault encrypt vault.yml
```

3. Set your NSX managers:

```bash
vim inventory.yml
```

## Mode 1: Report Only

```bash
ansible-playbook nsx_backup_manager.yml -i inventory.yml --ask-vault-pass
```

Outputs:
- `./reports/nsx_backup_data.json`
- `./reports/nsx_backup_report.csv`

## Mode 2: Configure Backup Policy

1. Edit `backup_policies.yml`.
2. Run with explicit confirmation flag:

```bash
ansible-playbook nsx_backup_manager.yml -i inventory.yml --ask-vault-pass \
  -e "nsx_operation=configure nsx_confirm_configure=true" \
  -e @backup_policies.yml
```

This applies the policy on each manager, then collects and writes the report artifacts.

## Policy Model

- `nsx_backup_policy_default`: baseline applied to all managers.
- `nsx_backup_policy_by_host`: optional per-manager overrides keyed by inventory hostname.

Per-manager value is merged recursively over default.

## Notes

- Set `nsx_validate_certs: true` in `inventory.yml` for trusted TLS in production.
- Keep secrets (for example remote backup server password) out of plaintext files; prefer vault-encrypted variables.
