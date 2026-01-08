import ssl
import json
import yaml
import atexit
from datetime import datetime, timezone

from pyVim.connect import SmartConnect, Disconnect
from pyVmomi import vim


def connect_vcenter(host, user, password, port=443, insecure=True):
    """Connect to a vCenter server."""
    context = None
    if insecure:
        context = ssl._create_unverified_context()
    si = SmartConnect(host=host, user=user, pwd=password, port=port, sslContext=context)
    atexit.register(Disconnect, si)
    return si


def get_all_hosts(si):
    """
    Retrieve all ESXi hosts from the vCenter inventory.
    Uses ContainerView pattern over vim.HostSystem [web:34].
    """
    view = si.content.viewManager.CreateContainerView(
        si.content.rootFolder, [vim.HostSystem], True
    )
    hosts = list(view.view)
    view.Destroy()
    return hosts


def normalize_version(v):
    """Normalize version string."""
    if not v:
        return ""
    return str(v).strip()


def build_vcenter_row(vcenter_name, hosts, target_version):
    """
    Build a summary row for a single vCenter:
    - total_hosts
    - upgrade_completed_total (count of hosts at target ESXi version)
    - completion_percentage
    """
    total_hosts = len(hosts)
    upgraded = 0

    for h in hosts:
        version = ""
        try:
            # vim.HostSystem is the managed object representing an ESXi host [web:34]
            version = normalize_version(h.config.product.version)
        except Exception:
            version = ""

        if version == target_version:
            upgraded += 1

    pct = round((upgraded / total_hosts) * 100, 1) if total_hosts else 0.0

    return {
        "vcenter": vcenter_name,
        "total_hosts": total_hosts,
        "upgrade_completed_total": upgraded,
        "completion_percentage": pct,
    }


def main():
    # Load vCenter config
    with open("config/vcenters.yaml", "r") as f:
        cfg = yaml.safe_load(f)

    target_version = normalize_version(cfg["target_esxi_version"])

    out = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "target_esxi_version": target_version,
        "rows": [],
    }

    # Connect to each vCenter and collect host data
    for vc in cfg["vcenters"]:
        try:
            si = connect_vcenter(vc["host"], vc["user"], vc["password"], insecure=True)
            hosts = get_all_hosts(si)
            out["rows"].append(build_vcenter_row(vc["name"], hosts, target_version))
            print(f"✓ {vc['name']}: {len(hosts)} hosts")
        except Exception as e:
            print(f"✗ {vc['name']}: {e}")

    # Write JSON for dashboard
    with open("public/vcenters.json", "w") as f:
        json.dump(out, f, indent=2)

    print(f"\nWrote public/vcenters.json at {out['generated_at']}")


if __name__ == "__main__":
    main()