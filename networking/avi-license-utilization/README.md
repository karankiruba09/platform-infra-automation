# Avi License Utilization

## Files

- `inputs/inventory.ini.example`: example list of Avi controllers
- `inputs/inventory.ini`: local working inventory (gitignored)
- `inputs/vars.example.yml`: non-secret vars template
- `inputs/vars.yml`: your local vars (gitignored)
- `inputs/vault.yml.example`: encrypted secret vars template
- `inputs/vault.yml`: your encrypted secret vars (gitignored)
- `license_usage.yml`: playbook using `community.network.avi_api_session`
- `output/license_usage_report.json`: generated JSON output (gitignored)
- `output/license_usage_report.csv`: generated CSV output (gitignored)
- `ansible.cfg`: local Ansible paths/temp settings

## Create Vault File

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

- `vault_avi_password`

## Run

From this directory:

```bash
ansible-playbook -i inputs/inventory.ini license_usage.yml --ask-vault-pass
```

Notes:

- This version uses the `community.network` collection already present on your box.
- SSL cert verification is disabled by default in this module path, which matches your lab/internal requirement.
- `group_vars` was removed because this project only needs one small local vars file and one vault file; a flat layout is simpler here.
