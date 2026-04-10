# NSX-T DFW License Utilization

## Files
- `inputs/inventory.ini.example`: example list of NSX managers
- `inputs/inventory.ini`: local working inventory (gitignored)
- `inputs/vars.example.yml`: non-secret vars template
- `inputs/vars.yml`: your local vars (gitignored)
- `inputs/vault.yml.example`: encrypted secret vars template
- `inputs/vault.yml`: your encrypted secret vars (gitignored)
- `license_usage.yml`: playbook using the NSX-T `licenses-usage` API
- `output/license_usage_report.json`: generated JSON output (gitignored)
- `output/license_usage_report.csv`: generated CSV output (gitignored)
- `ansible.cfg`: local Ansible temp settings

## Create Local Files
From this directory:

```bash
cp inputs/inventory.ini.example inputs/inventory.ini
cp inputs/vars.example.yml inputs/vars.yml
cp inputs/vault.yml.example inputs/vault.yml
ansible-vault encrypt inputs/vault.yml
```

Edit encrypted values:

```bash
ansible-vault edit inputs/vault.yml
```

Set:
- `vault_nsx_password`

## Run
From this directory:

```bash
ansible-playbook -i inputs/inventory.ini license_usage.yml --ask-vault-pass
```

## API
This project queries:
- `GET /api/v1/licenses/licenses-usage`

The summary extracts the `DFW` feature row and reports:
- `vm_usage_count`
- `cpu_usage_count`
- `ccu_usage_count`
- `vcpu_usage_count`
- `core_usage_count`

Notes:
- Basic Auth is used for simplicity.
- `validate_certs` defaults to `false` in `inputs/vars.yml` for lab/internal environments.
