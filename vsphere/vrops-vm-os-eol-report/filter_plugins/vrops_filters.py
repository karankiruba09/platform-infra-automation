"""Custom Ansible filters for vROps VM OS EOL report."""


class FilterModule:
    def filters(self):
        return {
            "vrops_parse_properties": self.vrops_parse_properties,
            "vrops_classify_vms": self.vrops_classify_vms,
        }

    @staticmethod
    def vrops_parse_properties(prop_results, vm_id_name_map):
        """Parse bulk property query results into a list of {name, guest_os} dicts.

        Only includes powered-on VMs.
        """
        results = []
        for batch in prop_results:
            for entry in batch.get("json", {}).get("values", []):
                rid = entry.get("resourceId", "")
                vm_name = vm_id_name_map.get(rid, "unknown")
                guest_os = ""
                guest_id = ""
                power_state = ""
                pc = entry.get("property-contents", {})
                for prop in pc.get("property-content", []):
                    stat_key = prop.get("statKey", "")
                    vals = prop.get("values", [])
                    if stat_key == "config|guestFullName" and vals:
                        guest_os = str(vals[0])
                    elif stat_key == "config|guestId" and vals and not guest_os:
                        guest_id = str(vals[0])
                    elif stat_key == "summary|runtime|powerState" and vals:
                        power_state = str(vals[0]).lower()
                # Only include powered-on VMs
                if power_state in ("powered on", "poweredon", "1"):
                    results.append({
                        "name": vm_name,
                        "guest_os": guest_os if guest_os else guest_id,
                    })
        return results

    @staticmethod
    def vrops_classify_vms(vm_list, os_eol_map, category_rules=None):
        """Classify each VM by category and EOL status. Returns list of classified dicts."""
        if category_rules is None:
            category_rules = []

        results = []
        for vm in vm_list:
            os_name = vm.get("guest_os", "") or ""
            os_key = os_name if os_name else "none"

            if os_key in os_eol_map:
                cat = os_eol_map[os_key]["category"]
                status = os_eol_map[os_key]["status"]
            else:
                cat = "Linux"
                status = "Supported"
                os_lower = os_name.lower()
                for rule in category_rules:
                    if rule["keyword"].lower() in os_lower:
                        cat = rule["category"]
                        status = rule.get("status", "Supported")
                        break

            results.append({
                "name": vm.get("name", "unknown"),
                "guest_os": os_key,
                "category": cat,
                "eol_status": status,
            })
        return results
