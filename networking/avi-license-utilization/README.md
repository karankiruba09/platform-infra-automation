# Avi License Utilization

## Files

- `inventory.ini.example`: example list of Avi controllers
- `inventory.ini`: local working inventory (gitignored)
- `license_usage.yml`: playbook using `community.network.avi_api_session`
- `vars.yml`: non-secret vars
- `vault.yml`: encrypted secret vars (create this)
- `reports/license_usage_report.json`: generated JSON output
- `reports/license_usage_report.csv`: generated CSV output
- `ansible.cfg`: local Ansible paths/temp settings

## Create Vault File

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

- `vault_avi_password`

## Run

From this directory:

```bash
ansible-playbook -i inventory.ini license_usage.yml --ask-vault-pass
```

Notes:

- This version uses the `community.network` collection already present on your box.
- SSL cert verification is disabled by default in this module path, which matches your lab/internal requirement.
- `group_vars` was removed because this project only needs one small local vars file and one vault file; a flat layout is simpler here.
