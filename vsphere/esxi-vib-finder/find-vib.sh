#!/usr/bin/env bash
# Find ESXi hosts with a specific VIB installed across multiple vCenters.
# Uses govc (VMware govmomi CLI) to query VIBs via the vCenter API — no SSH required.
#
# Architecture: two-phase flat pool
#   Phase 1 — enumerate hosts from all vCenters in parallel (fast)
#   Phase 2 — check VIBs on ALL hosts in a single global worker pool (max throughput)
#
# Usage:
#   ./find-vib.sh [path/to/vcenters.txt]
#
# Default vcenter list: ./inputs/vcenters.txt
# Default output dir:   ./output
#
# Environment variables:
#   GOVC_USERNAME          vCenter username (prompted if unset)
#   GOVC_PASSWORD          vCenter password (prompted if unset)
#   VIB_NAME               VIB to search for (default: smx-provider)
#   MAX_WORKERS            global concurrent host checks (default: 50)
#   GOVC_INSECURE          skip TLS verification (default: true)
#   OUTPUT_DIR             results directory (default: ./output)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_PATH="${BASH_SOURCE[0]}"

# ── Configuration ──────────────────────────────────────────────────────────────
VIB_NAME="${VIB_NAME:-smx-provider}"
MAX_WORKERS="${MAX_WORKERS:-50}"
OUTPUT_DIR="${OUTPUT_DIR:-${SCRIPT_DIR}/output}"
export GOVC_INSECURE="${GOVC_INSECURE:-true}"

# ── Internal dispatch (self-invocation targets for xargs -P) ──────────────────

# Phase 1 worker: enumerate hosts for one vCenter
if [[ "${1:-}" == "--enum" ]]; then
    vc_addr="$2"; work_dir="$3"
    url="$vc_addr"
    [[ "$url" == https://* ]] || url="https://${url}/sdk"
    export GOVC_URL="$url"

    err_file=$(mktemp)
    if hosts=$(govc find / -type h 2>"$err_file"); then
        if [[ -n "$hosts" ]]; then
            # Write vcenter<TAB>host_path lines to a per-vc file
            echo "$hosts" | while read -r h; do
                printf '%s\t%s\n' "$vc_addr" "$h"
            done > "${work_dir}/hosts.${vc_addr}"
            count=$(echo "$hosts" | wc -l | tr -d ' ')
            echo "[enum] ${vc_addr} — ${count} hosts"
        else
            echo "[enum] ${vc_addr} — 0 hosts"
        fi
    else
        echo "${vc_addr},-,-,VC_ERROR,,,," > "${work_dir}/.result.vc_err.${vc_addr}"
        echo "[enum] ${vc_addr} — FAILED: $(cat "$err_file")" >&2
    fi
    rm -f "$err_file"
    exit 0
fi

# Phase 2 worker: check VIBs on one host
if [[ "${1:-}" == "--check" ]]; then
    # Reads one line from stdin: vcenter<TAB>host_path
    vc_addr="$2"; host_path="$3"; work_dir="$4"; vib="$5"
    url="$vc_addr"
    [[ "$url" == https://* ]] || url="https://${url}/sdk"
    export GOVC_URL="$url"
    host_short="${host_path##*/}"

    tmp_result=$(mktemp "${work_dir}/.result.XXXXXX")

    if result=$(govc host.esxcli -host "$host_path" -- software vib list 2>/dev/null); then
        match=$(echo "$result" | grep -i "$vib" || true)
        if [[ -n "$match" ]]; then
            vib_name=$(echo "$match" | awk '{print $1}')
            vib_version=$(echo "$match" | awk '{print $2}')
            vib_vendor=$(echo "$match" | awk '{print $3}')
            vib_date=$(echo "$match" | awk '{print $5}')
            echo "${vc_addr},${host_short},${host_path},FOUND,${vib_name},${vib_version},${vib_vendor},${vib_date}" > "$tmp_result"
            echo "  [FOUND] ${vc_addr} / ${host_short} — ${vib_name} ${vib_version}"
        else
            rm -f "$tmp_result"
        fi
    else
        echo "${vc_addr},${host_short},${host_path},ERROR,,,," > "$tmp_result"
        echo "  [ERROR] ${vc_addr} / ${host_short}" >&2
    fi
    exit 0
fi

# ── Main entry point ──────────────────────────────────────────────────────────

VCENTER_FILE="${1:-${SCRIPT_DIR}/inputs/vcenters.txt}"

if [[ ! -f "$VCENTER_FILE" ]]; then
    echo "Error: vCenter list not found: ${VCENTER_FILE}" >&2
    echo "Create a file with one vCenter address per line (comments and blanks are ignored)." >&2
    exit 1
fi

if ! command -v govc &>/dev/null; then
    echo "Error: govc not found in PATH." >&2
    exit 1
fi

# Prompt for credentials if not set
if [[ -z "${GOVC_USERNAME:-}" ]]; then
    read -rp "vCenter username: " GOVC_USERNAME
fi
if [[ -z "${GOVC_PASSWORD:-}" ]]; then
    read -rsp "vCenter password: " GOVC_PASSWORD
    echo
fi
export GOVC_USERNAME GOVC_PASSWORD

# Parse vCenter list
mapfile -t VCENTERS < <(grep -v '^\s*#' "$VCENTER_FILE" | grep -v '^\s*$' | sed 's/\s*$//')

if [[ ${#VCENTERS[@]} -eq 0 ]]; then
    echo "Error: no vCenters found in ${VCENTER_FILE}" >&2
    exit 1
fi

# Prepare working directory
mkdir -p "$OUTPUT_DIR"
WORK_DIR=$(mktemp -d "${OUTPUT_DIR}/.work.XXXXXX")
RESULTS_FILE="${OUTPUT_DIR}/results.csv"
SUMMARY_FILE="${OUTPUT_DIR}/summary.txt"
echo "vcenter,host,host_path,status,vib_name,vib_version,vib_vendor,install_date" > "$RESULTS_FILE"

trap 'rm -rf "$WORK_DIR"' EXIT

echo "════════════════════════════════════════════════════════════════════"
echo " VIB Scanner — searching for '${VIB_NAME}'"
echo " vCenters: ${#VCENTERS[@]} | global workers: ${MAX_WORKERS}"
echo "════════════════════════════════════════════════════════════════════"
echo ""

START_TIME=$(date +%s)

# ── Phase 1: Enumerate hosts from ALL vCenters in parallel ────────────────────
echo "── Phase 1: Enumerating hosts from ${#VCENTERS[@]} vCenters ──"
printf '%s\n' "${VCENTERS[@]}" | xargs -I{} -P "${#VCENTERS[@]}" \
    bash "$SCRIPT_PATH" --enum {} "$WORK_DIR"

# Merge host lists into a single manifest
MANIFEST="${WORK_DIR}/manifest.tsv"
cat "${WORK_DIR}"/hosts.* > "$MANIFEST" 2>/dev/null || true
TOTAL_HOSTS=$(wc -l < "$MANIFEST" 2>/dev/null | tr -d ' ' || echo 0)

ENUM_TIME=$(( $(date +%s) - START_TIME ))
echo ""
echo "── Phase 2: Checking ${TOTAL_HOSTS} hosts (${MAX_WORKERS} workers) ──"

# ── Phase 2: Check VIBs on all hosts in a flat global pool ────────────────────
if [[ "$TOTAL_HOSTS" -gt 0 ]]; then
    while IFS=$'\t' read -r vc_addr host_path; do
        printf '%s\t%s\n' "$vc_addr" "$host_path"
    done < "$MANIFEST" | xargs -I{} -P "$MAX_WORKERS" \
        bash -c 'vc="${1%%	*}"; hp="${1#*	}"; exec bash "'"$SCRIPT_PATH"'" --check "$vc" "$hp" "'"$WORK_DIR"'" "'"$VIB_NAME"'"' _ {}
fi

END_TIME=$(date +%s)
ELAPSED=$(( END_TIME - START_TIME ))

# ── Merge results ─────────────────────────────────────────────────────────────
for f in "${WORK_DIR}"/.result.*; do
    [[ -f "$f" ]] && cat "$f" >> "$RESULTS_FILE"
done

# ── Summary ───────────────────────────────────────────────────────────────────
FOUND_COUNT=$(grep ',FOUND,' "$RESULTS_FILE" 2>/dev/null | wc -l | tr -d ' ')
HOST_ERROR_COUNT=$(grep ',ERROR,' "$RESULTS_FILE" 2>/dev/null | grep -v ',VC_ERROR,' | wc -l | tr -d ' ')
VC_ERROR_COUNT=$(grep ',VC_ERROR,' "$RESULTS_FILE" 2>/dev/null | wc -l | tr -d ' ')
VC_OK=$(( ${#VCENTERS[@]} - VC_ERROR_COUNT ))

{
    echo ""
    echo "════════════════════════════════════════════════════════════════════"
    echo " VIB Scan Summary — $(date '+%Y-%m-%d %H:%M:%S')"
    echo "════════════════════════════════════════════════════════════════════"
    echo ""
    echo " VIB searched:       ${VIB_NAME}"
    echo " vCenters OK/total:  ${VC_OK}/${#VCENTERS[@]}"
    echo " Hosts scanned:      ${TOTAL_HOSTS}"
    echo " Hosts with VIB:     ${FOUND_COUNT}"
    [[ "$HOST_ERROR_COUNT" -gt 0 ]] && \
    echo " Host query errors:  ${HOST_ERROR_COUNT}"
    [[ "$VC_ERROR_COUNT" -gt 0 ]] && \
    echo " vCenter failures:   ${VC_ERROR_COUNT}"
    echo " Enumeration time:   ${ENUM_TIME}s"
    echo " Total time:         ${ELAPSED}s"
    echo ""

    if [[ "$FOUND_COUNT" -gt 0 ]]; then
        echo "────────────────────────────────────────────────────────────────────"
        echo " Hosts with '${VIB_NAME}' installed:"
        echo "────────────────────────────────────────────────────────────────────"
        echo ""
        printf "  %-45s %-30s %-20s %s\n" "VCENTER" "HOST" "VERSION" "INSTALL DATE"
        printf "  %-45s %-30s %-20s %s\n" "───────" "────" "───────" "────────────"
        grep ',FOUND,' "$RESULTS_FILE" | sort -t',' -k1,1 -k2,2 | \
            while IFS=',' read -r vc host _ _ vib_name vib_ver _ vib_date; do
                printf "  %-45s %-30s %-20s %s\n" "$vc" "$host" "$vib_ver" "$vib_date"
            done
        echo ""
    else
        echo " No hosts found with '${VIB_NAME}' installed."
        echo ""
    fi

    if [[ "$VC_ERROR_COUNT" -gt 0 ]]; then
        echo "────────────────────────────────────────────────────────────────────"
        echo " vCenters that failed to connect:"
        echo "────────────────────────────────────────────────────────────────────"
        echo ""
        grep ',VC_ERROR,' "$RESULTS_FILE" | while IFS=',' read -r vc _rest; do
            echo "  ${vc}"
        done
        echo ""
    fi

    if [[ "$HOST_ERROR_COUNT" -gt 0 ]]; then
        echo "────────────────────────────────────────────────────────────────────"
        echo " Hosts with errors (could not query VIBs):"
        echo "────────────────────────────────────────────────────────────────────"
        echo ""
        grep ',ERROR,' "$RESULTS_FILE" | grep -v ',VC_ERROR,' | \
            while IFS=',' read -r vc host _rest; do
                echo "  ${vc} / ${host}"
            done
        echo ""
    fi
} | tee "$SUMMARY_FILE"

echo "Full CSV:  ${RESULTS_FILE}"
echo "Summary:   ${SUMMARY_FILE}"
