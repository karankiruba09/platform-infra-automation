# NSX-T DFW License Utilization

## Files
- `inventory.ini.example`: example list of NSX managers
- `inventory.ini`: local working inventory (gitignored)
- `license_usage.yml`: playbook using the NSX-T `licenses-usage` API
- `vars.yml`: non-secret vars
- `vault.yml`: encrypted secret vars (create this)
- `reports/license_usage_report.json`: generated JSON output
- `reports/license_usage_report.csv`: generated CSV output
- `ansible.cfg`: local Ansible temp settings

## Create Local Files
From this directory:

```bash
cp inventory.ini.example inventory.ini
cp vault.yml.example vault.yml
ansible-vault encrypt vault.yml
```

Edit encrypted values:

```bash
ansible-vault edit vault.yml
```

Set:
- `vault_nsx_password`

## Run
From this directory:

```bash
ansible-playbook -i inventory.ini license_usage.yml --ask-vault-pass
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
- `validate_certs` defaults to `false` in `vars.yml` for lab/internal environments.
