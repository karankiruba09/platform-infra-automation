# NSX Manager Backup Automation

Single Ansible project with two functions:

1. `report` mode: collect current NSX backup configuration and latest backup status from all NSX managers.
2. `configure` mode: apply backup configuration from an input policy file.

## Files

```text
nsx-config-backup-manager/
├── nsx_backup_manager.yml              # Single playbook (report + configure)
├── inputs/                             # gitignored except *.example*
│   ├── inventory.example.yml           # NSX manager list template
│   ├── backup_policies.example.yml     # Configure-mode policy template
│   └── vault.yml.example               # Credentials template
├── output/                             # Generated report artifacts (gitignored)
├── templates/
│   └── nsx_backup_report.csv.j2
└── requirements.txt
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

1. Prepare credentials:

```bash
cp inputs/vault.yml.example inputs/vault.yml
vim inputs/vault.yml
ansible-vault encrypt inputs/vault.yml
```

1. Set your NSX managers:

```bash
cp inputs/inventory.example.yml inputs/inventory.yml
vim inputs/inventory.yml
```

## Mode 1: Report Only

```bash
ansible-playbook nsx_backup_manager.yml -i inputs/inventory.yml --ask-vault-pass
```

Outputs:

- `./output/nsx_backup_data.json`
- `./output/nsx_backup_report.csv`

## Mode 2: Configure Backup Policy

1. Copy and edit `inputs/backup_policies.example.yml` → `inputs/backup_policies.yml`.
2. Run with explicit confirmation flag:

```bash
ansible-playbook nsx_backup_manager.yml -i inputs/inventory.yml --ask-vault-pass \
  -e "nsx_operation=configure nsx_confirm_configure=true" \
  -e @inputs/backup_policies.yml
```

This applies the policy on each manager, then collects and writes the report artifacts.

## Policy Model

- `nsx_backup_policy_default`: baseline applied to all managers.
- `nsx_backup_policy_by_host`: optional per-manager overrides keyed by inventory hostname.

Per-manager value is merged recursively over default.

## Notes

- Set `nsx_validate_certs: true` in `inputs/inventory.yml` for trusted TLS in production.
- Keep secrets (for example remote backup server password) out of plaintext files; prefer vault-encrypted variables.
