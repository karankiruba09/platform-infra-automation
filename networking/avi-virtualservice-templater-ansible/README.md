# Avi VS Creation Standard (YAML Inputs, Single Playbook)

Single playbook for creating/updating new Avi Virtual Services from one standard template.
The current template keeps only settings that matched on both reference VS objects; non-common fields are exposed in the values file.

You only edit one YAML values file for each new VS.

## Files

```text
avi-virtualservice-templater-ansible/
├── playbooks/
│   └── create_virtual_service.yml
├── scripts/
│   └── deploy_bundle.sh
├── inputs/
│   ├── standard-vs-template.yaml.j2
│   ├── standard-vs-variable-catalog.yaml
│   ├── new-vs.values.example.yaml
│   └── controller.env.example
├── ansible.cfg
├── inventory.yml
└── artifacts/
```

## What to edit

```bash
cd networking/avi-virtualservice-templater-ansible
cp inputs/new-vs.values.example.yaml inputs/new-vs.values.yaml
```

Edit only `inputs/new-vs.values.yaml`.

Required keys:
- `virtual_service_name`
- `vsvip_name`
- `vip_address`
- `pool_name`
- `backend_server_ips`
- `service_port`
- `backend_server_port`
- `health_monitor_name`
- `health_monitor_type`
- `network_profile_ref`
- `se_group_ref`
- `vs_analytics_full_client_logs_duration`
- `network_security_policy_ref`

Optional keys:
- `cloud_ref` (default: from template)
- `vrf_ref` (default: from template)
- `vrf_context_ref` (default: same as `vrf_ref`)
- `pool_name`
- `health_monitor_monitor_port`
- `health_monitor_https_http_request`
- `health_monitor_ssl_profile_ref`
- `health_monitor_external_command_code`
- `health_monitor_external_command_variables`
- `redis_db`
- `redis_port`

## Run

```bash
cd networking/avi-virtualservice-templater-ansible
ansible-playbook playbooks/create_virtual_service.yml \
  -e avi_controller='<AVI_CONTROLLER_FQDN>' \
  -e avi_username='<AVI_USERNAME>' \
  -e avi_password='<AVI_PASSWORD>' \
  -e avi_tenant='<AVI_TENANT>' \
  -e avi_api_version='<AVI_API_VERSION>' \
  -e avi_validate_certs='<true_or_false>' \
  -e standard_template_file=inputs/standard-vs-template.yaml.j2 \
  -e new_vs_values_file=inputs/new-vs.values.yaml
```

## Behavior

- Session auth (`/login`) is used.
- Idempotent upsert order:
  1. health monitors
  2. ssl profiles
  3. pool
  4. vsvip
  5. virtual service

`se_group_ref` points to an existing Service Engine Group. Service Engines themselves are runtime objects managed by Avi and are not created directly by this automation.

## Why template still has many fixed values

That is intentional: only fields that matched across the two reference VS objects are fixed in the standard template.
Everything that differed is in `new-vs.values.yaml`.
