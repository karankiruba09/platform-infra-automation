#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

usage() {
  cat <<'EOF'
Usage: ./scripts/collect.sh [--debug-keep-tmp]

Collect ESXi host versions across vCenters listed in config/vcenters.txt and
write JSON+CSV artifacts into ./public.

Options:
  --debug-keep-tmp   Keep the temporary working directory and print its path
EOF
}

if [[ -f ".env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source ".env"
  set +a
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing dependency: $1" >&2
    exit 2
  fi
}

require_cmd govc
require_cmd jq
require_cmd timeout

VCENTERS_FILE="${VCENTERS_FILE:-$ROOT_DIR/config/vcenters.txt}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/public}"
REPORT_JSON="${REPORT_JSON:-$OUT_DIR/esxi_versions.json}"
HOSTS_CSV="${HOSTS_CSV:-$OUT_DIR/esxi_hosts.csv}"
HOSTS_JSON="${HOSTS_JSON:-$OUT_DIR/esxi_hosts.json}"

DEBUG_KEEP_TMP="${DEBUG_KEEP_TMP:-false}"

VC_USER="${VC_USER:-}"
VC_PASSWORD="${VC_PASSWORD:-}"
VC_INSECURE="${VC_INSECURE:-true}"
VC_PARALLEL="${VC_PARALLEL:-20}"
VC_TIMEOUT_SECONDS="${VC_TIMEOUT_SECONDS:-12}"
VC_UNSET_PROXY="${VC_UNSET_PROXY:-true}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --debug-keep-tmp)
      DEBUG_KEEP_TMP=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$VC_USER" || -z "$VC_PASSWORD" ]]; then
  echo "ERROR: VC_USER and VC_PASSWORD must be set (in .env or environment)" >&2
  exit 2
fi

if [[ ! -f "$VCENTERS_FILE" ]]; then
  echo "ERROR: vCenter list not found: $VCENTERS_FILE" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"

TMP_DIR="$(mktemp -d)"
case "${DEBUG_KEEP_TMP,,}" in
  1|true|yes|y)
    trap 'echo "DEBUG_KEEP_TMP enabled: temp dir preserved at $TMP_DIR" >&2' EXIT
    ;;
  *)
    trap 'rm -rf "$TMP_DIR"' EXIT
    ;;
esac

generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

vc_list="$TMP_DIR/vcenters.list"
sed -e 's/\r$//' "$VCENTERS_FILE" | awk '
  /^[[:space:]]*#/ {next}
  /^[[:space:]]*$/ {next}
  {print}
' > "$vc_list"

if [[ ! -s "$vc_list" ]]; then
  echo "ERROR: no vCenters found in $VCENTERS_FILE" >&2
  exit 2
fi

if ! [[ "$VC_PARALLEL" =~ ^[0-9]+$ ]] || (( VC_PARALLEL < 1 )); then
  echo "ERROR: VC_PARALLEL must be a positive integer (got: $VC_PARALLEL)" >&2
  exit 2
fi
if ! [[ "$VC_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || (( VC_TIMEOUT_SECONDS < 1 )); then
  echo "ERROR: VC_TIMEOUT_SECONDS must be a positive integer (got: $VC_TIMEOUT_SECONDS)" >&2
  exit 2
fi

collect_one_vcenter() {
  local line name host safe_name raw_json row_json hosts_json hosts_csv err_file url
  local start_ms end_ms duration_ms rc err_msg
  local -a env_unset_proxy=()

  line="$1"

  name="$line"
  host="$line"
  if [[ "$line" == *"|"* ]]; then
    name="${line%%|*}"
    host="${line#*|}"
  fi

  name="$(echo "$name" | awk '{gsub(/^[[:space:]]+|[[:space:]]+$/, ""); print}')"
  host="$(echo "$host" | awk '{gsub(/^[[:space:]]+|[[:space:]]+$/, ""); print}')"

  safe_name="$(printf '%s' "$name" | tr -c 'A-Za-z0-9._-' '_')"
  raw_json="$TMP_DIR/${safe_name}.raw.json"
  row_json="$TMP_DIR/${safe_name}.row.json"
  hosts_json="$TMP_DIR/${safe_name}.hosts.json"
  hosts_csv="$TMP_DIR/${safe_name}.hosts.csv"
  err_file="$TMP_DIR/${safe_name}.err"

  url="$host"
  if [[ "$url" == http*://* ]]; then
    # Normalize to vCenter SDK endpoint if the user provided only scheme+host.
    if [[ "$url" != */sdk* ]]; then
      url="${url%/}/sdk"
    fi
  else
    url="https://${url}/sdk"
  fi

  case "${VC_UNSET_PROXY,,}" in
    1|true|yes|y)
      env_unset_proxy=(-u http_proxy -u https_proxy -u HTTP_PROXY -u HTTPS_PROXY -u all_proxy -u ALL_PROXY)
      ;;
  esac

  start_ms="$(date +%s%3N)"

  set +e
  timeout "${VC_TIMEOUT_SECONDS}s" env "${env_unset_proxy[@]}" \
    GOVC_URL="$url" \
    GOVC_USERNAME="$VC_USER" \
    GOVC_PASSWORD="$VC_PASSWORD" \
    GOVC_INSECURE="$VC_INSECURE" \
    GOVC_PERSIST_SESSION=true \
    govc object.collect -json -type h / \
      name \
      config.product.version \
      config.product.build \
      runtime.connectionState \
      runtime.inMaintenanceMode \
    >"$raw_json" 2>"$err_file"
  rc=$?
  set -e

  end_ms="$(date +%s%3N)"
  duration_ms="$((end_ms - start_ms))"

  if [[ $rc -ne 0 ]]; then
    err_msg="$(tr "\n" " " <"$err_file" | sed 's/[[:space:]]\\+/ /g' | cut -c1-4000)"
    jq -n \
      --arg vcenter "$name" \
      --arg source "$host" \
      --arg collected_at "$generated_at" \
      --arg error "$err_msg" \
      --argjson duration_ms "$duration_ms" \
      '{
        vcenter: $vcenter,
        source: $source,
        status: "error",
        error: $error,
        collected_at: $collected_at,
        duration_ms: $duration_ms,
        total_hosts: 0,
        connected_hosts: 0,
        maintenance_hosts: 0,
        major_counts: {"8.x":0,"7.x":0,"6.x":0,"other":0,"unknown":0},
        build_breakdown_8x: [],
        counts: {"8.0u3":0, "8.0u2":0, "older":0, "unknown":0}
      }' >"$row_json"
    echo "[]" >"$hosts_json"
    : >"$hosts_csv"
    return 0
  fi

  if ! jq -c -s --arg vc "$name" '
    def pset($o):
      ($o.propSet // $o.PropSet // []);

    def prop($o; $n):
      (pset($o) | map(select((.name // .Name) == $n) | (.val // .Val)) | .[0]);

    def group_for($ver):
      if (($ver // "") | tostring | startswith("8.0.3")) then "8.0u3"
      elif (($ver // "") | tostring | startswith("8.0.2")) then "8.0u2"
      elif ($ver == null or $ver == "") then "unknown"
      else "older"
      end;

    def major_for($ver):
      if (($ver // "") | tostring | startswith("8.")) then "8.x"
      elif (($ver // "") | tostring | startswith("7.")) then "7.x"
      elif (($ver // "") | tostring | startswith("6.")) then "6.x"
      elif ($ver == null or $ver == "") then "unknown"
      else "other"
      end;

    def as_host($o):
      {
        host: (prop($o; "name") // "unknown" | tostring),
        version: (prop($o; "config.product.version") // null),
        build: (prop($o; "config.product.build") // null),
        connection_state: (prop($o; "runtime.connectionState") // null),
        maintenance_mode: (prop($o; "runtime.inMaintenanceMode") // false)
      }
      | .version = (if .version == null then null else (.version | tostring) end)
      | .build = (if .build == null then null else (.build | tostring) end)
      | .connection_state = (if .connection_state == null then null else (.connection_state | tostring) end)
      | .maintenance_mode = (if .maintenance_mode == null then false else .maintenance_mode end)
      | .group = group_for(.version)
      | .major = major_for(.version)
      | .vcenter = $vc;

    def normalize_doc($d):
      if ($d|type)=="array" then $d
      elif ($d|type)=="object" and ($d.elements|type)=="array" then $d.elements
      elif ($d|type)=="object" and ($d.Elements|type)=="array" then $d.Elements
      elif ($d|type)=="object" and ($d.objects|type)=="array" then $d.objects
      elif ($d|type)=="object" and ($d.Objects|type)=="array" then $d.Objects
      else [$d]
      end;

    def normalize_prop($o):
      # govc can emit ObjectUpdate items: {kind,obj,changeSet:[{name,val}...]}.
      # Normalize these into our expected {obj,propSet:[...]} form.
      if ($o|type)=="object" and (has("changeSet")) and (($o.changeSet|type)=="array") and (has("propSet")|not) then
        ($o + {propSet: $o.changeSet} | del(.changeSet))
      elif ($o|type)=="object" and (has("ChangeSet")) and (($o.ChangeSet|type)=="array") and (has("propSet")|not) then
        ($o + {propSet: $o.ChangeSet} | del(.ChangeSet))
      elif ($o|type)=="object" and (has("PropSet")) and (has("propSet")|not) then
        ($o + {propSet: $o.PropSet} | del(.PropSet))
      else $o end;

    def direct_candidates:
      reduce .[] as $d ([]; . + (normalize_doc($d)))
      | map(select(type=="object"))
      | map(normalize_prop(.))
      | map(select(has("propSet") and (.propSet|type=="array")));

    def recursive_candidates:
      [ .[] | .. | objects | select(has("propSet") or has("PropSet")) ]
      | map(normalize_prop(.))
      | map(select(has("propSet") and (.propSet|type=="array")));

    (direct_candidates) as $c
    | (if ($c|length) > 0 then $c else recursive_candidates end)
    | [ .[] | as_host(.) ]
  ' "$raw_json" >"$hosts_json" 2>"$err_file"; then
    err_msg="$(tr "\n" " " <"$err_file" | sed 's/[[:space:]]\\+/ /g' | cut -c1-4000)"
    jq -n \
      --arg vcenter "$name" \
      --arg source "$host" \
      --arg collected_at "$generated_at" \
      --arg error "jq parse failed: $err_msg" \
      --argjson duration_ms "$duration_ms" \
      '{
        vcenter: $vcenter,
        source: $source,
        status: "error",
        error: $error,
        collected_at: $collected_at,
        duration_ms: $duration_ms,
        total_hosts: 0,
        connected_hosts: 0,
        maintenance_hosts: 0,
        major_counts: {"8.x":0,"7.x":0,"6.x":0,"other":0,"unknown":0},
        build_breakdown_8x: [],
        counts: {"8.0u3":0, "8.0u2":0, "older":0, "unknown":0}
      }' >"$row_json"
    echo "[]" >"$hosts_json"
    : >"$hosts_csv"
    return 0
  fi

  if ! jq -r --arg vc "$name" '
    .[] | [
      $vc,
      .host,
      (.major // "unknown"),
      (.version // ""),
      (.build // ""),
      (.group // "unknown"),
      (.connection_state // ""),
      (.maintenance_mode|tostring)
    ] | @csv
  ' "$hosts_json" >"$hosts_csv" 2>"$err_file"; then
    err_msg="$(tr "\n" " " <"$err_file" | sed 's/[[:space:]]\\+/ /g' | cut -c1-4000)"
    jq -n \
      --arg vcenter "$name" \
      --arg source "$host" \
      --arg collected_at "$generated_at" \
      --arg error "jq csv failed: $err_msg" \
      --argjson duration_ms "$duration_ms" \
      '{
        vcenter: $vcenter,
        source: $source,
        status: "error",
        error: $error,
        collected_at: $collected_at,
        duration_ms: $duration_ms,
        total_hosts: 0,
        connected_hosts: 0,
        maintenance_hosts: 0,
        major_counts: {"8.x":0,"7.x":0,"6.x":0,"other":0,"unknown":0},
        build_breakdown_8x: [],
        counts: {"8.0u3":0, "8.0u2":0, "older":0, "unknown":0}
      }' >"$row_json"
    echo "[]" >"$hosts_json"
    : >"$hosts_csv"
    return 0
  fi

  if ! jq -n \
    --arg vcenter "$name" \
    --arg source "$host" \
    --arg collected_at "$generated_at" \
    --argjson duration_ms "$duration_ms" \
    --slurpfile hosts "$hosts_json" \
    '
    def counts($hosts):
      reduce ($hosts[]? ) as $h ({"8.0u3":0,"8.0u2":0,"older":0,"unknown":0}; .[$h.group] += 1);

    def major_counts($hosts):
      reduce ($hosts[]? ) as $h ({"8.x":0,"7.x":0,"6.x":0,"other":0,"unknown":0}; .[($h.major // "unknown")] += 1);

    def build_breakdown_8x($hosts):
      ($hosts
        | map(select((.major // "") == "8.x"))
        | map({version: (.version // "unknown" | tostring), build: (.build // "unknown" | tostring)})
        | sort_by([.version, .build])
        | group_by([.version, .build])
        | map({version: .[0].version, build: .[0].build, hosts: length})
        | sort_by(-.hosts)
      );

    def connected($hosts):
      ($hosts | map(select(.connection_state == "connected")) | length);

    def maintenance($hosts):
      ($hosts | map(select(.maintenance_mode == true)) | length);

    ($hosts[0] // []) as $h
    | {
        vcenter: $vcenter,
        source: $source,
        status: "ok",
        collected_at: $collected_at,
        duration_ms: $duration_ms,
        total_hosts: ($h | length),
        connected_hosts: connected($h),
        maintenance_hosts: maintenance($h),
        major_counts: major_counts($h),
        build_breakdown_8x: build_breakdown_8x($h),
        counts: counts($h)
      }
    ' >"$row_json" 2>"$err_file"; then
    err_msg="$(tr "\n" " " <"$err_file" | sed 's/[[:space:]]\\+/ /g' | cut -c1-4000)"
    jq -n \
      --arg vcenter "$name" \
      --arg source "$host" \
      --arg collected_at "$generated_at" \
      --arg error "jq row failed: $err_msg" \
      --argjson duration_ms "$duration_ms" \
      '{
        vcenter: $vcenter,
        source: $source,
        status: "error",
        error: $error,
        collected_at: $collected_at,
        duration_ms: $duration_ms,
        total_hosts: 0,
        connected_hosts: 0,
        maintenance_hosts: 0,
        major_counts: {"8.x":0,"7.x":0,"6.x":0,"other":0,"unknown":0},
        build_breakdown_8x: [],
        counts: {"8.0u3":0, "8.0u2":0, "older":0, "unknown":0}
      }' >"$row_json"
    echo "[]" >"$hosts_json"
    : >"$hosts_csv"
    return 0
  fi
}

# Run collections in parallel, one govc call per vCenter.
running=0
while IFS= read -r line; do
  collect_one_vcenter "$line" &
  running=$((running + 1))

  if (( running >= VC_PARALLEL )); then
    wait -n || true
    running=$((running - 1))
  fi
done < "$vc_list"

while (( running > 0 )); do
  wait -n || true
  running=$((running - 1))
done

# Aggregate rows + hosts.
rows_json="$TMP_DIR/rows.json"
jq -s 'sort_by(.vcenter)' "$TMP_DIR"/*.row.json >"$rows_json"

hosts_json_all="$TMP_DIR/hosts.json"
jq -s '[ .[] | select(type=="array") | .[] ]' "$TMP_DIR"/*.hosts.json >"$hosts_json_all"

# Write artifacts.
{
  echo '"vcenter","host","major","version","build","upgrade_group","connection_state","maintenance_mode"'
  cat "$TMP_DIR"/*.hosts.csv || true
} >"$HOSTS_CSV"

cp "$hosts_json_all" "$HOSTS_JSON"

jq -n \
  --arg generated_at "$generated_at" \
  --slurpfile rows "$rows_json" \
  --slurpfile hosts "$hosts_json_all" \
  '
  def sum_counts($rows):
    reduce ($rows[]? ) as $r ({"8.0u3":0,"8.0u2":0,"older":0,"unknown":0};
      .["8.0u3"] += ($r.counts["8.0u3"] // 0)
      | .["8.0u2"] += ($r.counts["8.0u2"] // 0)
      | .["older"] += ($r.counts["older"] // 0)
      | .["unknown"] += ($r.counts["unknown"] // 0)
    );

  def sum_major_counts($rows):
    reduce ($rows[]? ) as $r ({"8.x":0,"7.x":0,"6.x":0,"other":0,"unknown":0};
      .["8.x"] += ($r.major_counts["8.x"] // 0)
      | .["7.x"] += ($r.major_counts["7.x"] // 0)
      | .["6.x"] += ($r.major_counts["6.x"] // 0)
      | .["other"] += ($r.major_counts["other"] // 0)
      | .["unknown"] += ($r.major_counts["unknown"] // 0)
    );

  def sum($rows; $k):
    reduce ($rows[]? ) as $r (0; . + ($r[$k] // 0));

  def version_breakdown($hosts):
    ($hosts
      | map(.version // "unknown")
      | sort
      | group_by(.)
      | map({version: .[0], hosts: length})
      | sort_by(-.hosts)
    );

  def build_breakdown_8x($hosts):
    ($hosts
      | map(select((.major // "") == "8.x"))
      | map({version: (.version // "unknown" | tostring), build: (.build // "unknown" | tostring)})
      | sort_by([.version, .build])
      | group_by([.version, .build])
      | map({version: .[0].version, build: .[0].build, hosts: length})
      | sort_by(-.hosts)
    );

  ($rows[0] // []) as $r
  | ($hosts[0] // []) as $h
  | {
      generated_at: $generated_at,
      totals: {
        vcenters_total: ($r | length),
        vcenters_ok: ($r | map(select(.status=="ok")) | length),
        vcenters_error: ($r | map(select(.status!="ok")) | length),
        hosts_total: sum($r; "total_hosts"),
        connected_hosts: sum($r; "connected_hosts"),
        maintenance_hosts: sum($r; "maintenance_hosts"),
        major_counts: sum_major_counts($r),
        counts: sum_counts($r)
      },
      version_breakdown: version_breakdown($h),
      build_breakdown_8x: build_breakdown_8x($h),
      rows: $r
    }
  ' >"$REPORT_JSON"

echo "Wrote $REPORT_JSON ($generated_at)"
echo "Wrote $HOSTS_CSV"

# Optional: render a static, server-side HTML report (no JavaScript required).
REPORT_HTML="${REPORT_HTML:-$OUT_DIR/report.html}"
if command -v python3 >/dev/null 2>&1; then
  if python3 "$ROOT_DIR/scripts/server.py" --render-html "$REPORT_HTML" >/dev/null 2>&1; then
    echo "Wrote $REPORT_HTML"
  else
    echo "WARN: failed to render HTML report: $REPORT_HTML" >&2
  fi
else
  echo "WARN: python3 not found; skipping HTML report render" >&2
fi
