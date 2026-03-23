#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Usage:
  deploy_bundle.sh \\
    --controller <fqdn-or-ip> \\
    --username <avi-username> \\
    --password <avi-password> \\
    --bundle-file <rendered-bundle.json> \\
    [--tenant admin] \\
    [--api-version 30.2.1] \\
    [--insecure]
USAGE
}

need_bin() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ERROR: required binary not found: $1" >&2
    exit 1
  }
}

need_bin curl
need_bin jq

CONTROLLER=""
USERNAME=""
PASSWORD=""
BUNDLE_FILE=""
TENANT="admin"
API_VERSION="30.2.1"
INSECURE="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --controller) CONTROLLER="$2"; shift 2 ;;
    --username) USERNAME="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --bundle-file) BUNDLE_FILE="$2"; shift 2 ;;
    --tenant) TENANT="$2"; shift 2 ;;
    --api-version) API_VERSION="$2"; shift 2 ;;
    --insecure) INSECURE="true"; shift 1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ERROR: unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$CONTROLLER" || -z "$USERNAME" || -z "$PASSWORD" || -z "$BUNDLE_FILE" ]]; then
  echo "ERROR: missing required arguments" >&2
  usage
  exit 1
fi

if [[ ! -f "$BUNDLE_FILE" ]]; then
  echo "ERROR: bundle file not found: $BUNDLE_FILE" >&2
  exit 1
fi

COOKIE_JAR="$(mktemp)"
trap 'rm -f "$COOKIE_JAR"' EXIT

curl_opts=(-sS -b "$COOKIE_JAR" -c "$COOKIE_JAR")
if [[ "$INSECURE" == "true" ]]; then
  curl_opts+=(-k)
fi

login() {
  local code
  code="$(curl "${curl_opts[@]}" -o /dev/null -w '%{http_code}' -X POST "https://${CONTROLLER}/login" --data-urlencode "username=${USERNAME}" --data-urlencode "password=${PASSWORD}")"
  [[ "$code" == "200" ]] || { echo "ERROR: login failed HTTP ${code}" >&2; exit 1; }
}

csrf_token() { awk '$6 == "csrftoken" {print $7}' "$COOKIE_JAR" | tail -n1; }

api_headers() {
  local csrf
  csrf="$(csrf_token)"
  printf '%s\n' "-H" "Accept: application/json" "-H" "Content-Type: application/json" "-H" "X-Avi-Version: ${API_VERSION}" "-H" "X-Avi-Tenant: ${TENANT}" "-H" "X-CSRFToken: ${csrf}"
}

urlencode() { jq -nr --arg v "$1" '$v|@uri'; }

api_get() {
  local path="$1"
  local -a headers
  mapfile -t headers < <(api_headers)
  curl "${curl_opts[@]}" "${headers[@]}" "https://${CONTROLLER}${path}"
}

api_post() {
  local path="$1" payload="$2"
  local -a headers
  mapfile -t headers < <(api_headers)
  curl "${curl_opts[@]}" "${headers[@]}" -X POST "https://${CONTROLLER}${path}" --data "$payload"
}

api_put() {
  local path="$1" payload="$2"
  local -a headers
  mapfile -t headers < <(api_headers)
  curl "${curl_opts[@]}" "${headers[@]}" -X PUT "https://${CONTROLLER}${path}" --data "$payload"
}

refs_to_query() {
  jq '
    def endpoint_for($k):
      if $k == "vrf_context_ref" then "vrfcontext"
      elif $k == "vrf_ref" then "vrfcontext"
      elif $k == "health_monitor_refs" then "healthmonitor"
      elif $k == "ssl_profile_ref" or $k == "ssl_profile_refs" then "sslprofile"
      elif $k == "ssl_key_and_certificate_ref" or $k == "ssl_key_and_certificate_refs" then "sslkeyandcertificate"
      elif $k == "application_profile_ref" then "applicationprofile"
      elif $k == "network_profile_ref" then "networkprofile"
      elif $k == "analytics_profile_ref" then "analyticsprofile"
      elif $k == "error_page_profile_ref" then "errorpageprofile"
      elif $k == "rewritable_content_ref" then "stringgroup"
      elif $k == "network_security_policy_ref" then "networksecuritypolicy"
      elif $k == "se_group_ref" then "serviceenginegroup"
      elif $k == "pool_ref" then "pool"
      elif $k == "cloud_ref" then "cloud"
      elif ($k | endswith("_ref")) then ($k | sub("_ref$"; "") | gsub("_"; ""))
      elif ($k | endswith("_refs")) then ($k | sub("_refs$"; "") | gsub("_"; ""))
      else ""
      end;

    def ref_to_query($k; $v):
      if ($v|type) != "string" then $v
      elif ($v | test("/api/")) then $v
      else ("/api/" + endpoint_for($k) + "?name=" + ($v|@uri))
      end;

    def conv:
      if type == "object" then
        to_entries
        | map(
            if (.key|endswith("_ref")) and (.value|type=="string") then .value = ref_to_query(.key; .value)
            elif (.key|endswith("_refs")) and (.value|type=="array") then .value = (.value | map(if type=="string" then ref_to_query(.key; .) else (.|conv) end))
            else .value = (.value | conv)
            end
          )
        | from_entries
      elif type == "array" then map(conv)
      else . end;

    conv
  '
}

upsert_by_name() {
  local endpoint="$1" payload="$2"
  local name lookup count uuid
  name="$(jq -r '.name // empty' <<<"$payload")"
  [[ -n "$name" ]] || { echo "ERROR: missing name for endpoint ${endpoint}" >&2; exit 1; }
  lookup="$(api_get "/api/${endpoint}?name=$(urlencode "$name")")"
  count="$(jq -r '.count // 0' <<<"$lookup")"
  if [[ "$count" -gt 0 ]]; then
    uuid="$(jq -r '.results[0].uuid' <<<"$lookup")"
    api_put "/api/${endpoint}/${uuid}" "$payload" >/dev/null
    echo "updated: ${endpoint}/${name}"
  else
    api_post "/api/${endpoint}" "$payload" >/dev/null
    echo "created: ${endpoint}/${name}"
  fi
}

login
RENDERED_BUNDLE="$(cat "$BUNDLE_FILE")"
RENDERED_BUNDLE="$(refs_to_query <<<"$RENDERED_BUNDLE")"
HM_LIST="$(jq -c '.health_monitors // []' <<<"$RENDERED_BUNDLE")"
SSL_LIST="$(jq -c '.ssl_profiles // []' <<<"$RENDERED_BUNDLE")"
POOL_OBJ="$(jq -c '.pool' <<<"$RENDERED_BUNDLE")"
VSVIP_OBJ="$(jq -c '.vsvip // empty' <<<"$RENDERED_BUNDLE")"
VS_OBJ="$(jq -c '.virtual_service' <<<"$RENDERED_BUNDLE")"
while IFS= read -r hm; do [[ -z "$hm" ]] && continue; upsert_by_name "healthmonitor" "$hm"; done < <(jq -c '.[]' <<<"$HM_LIST")
while IFS= read -r ssl; do [[ -z "$ssl" ]] && continue; upsert_by_name "sslprofile" "$ssl"; done < <(jq -c '.[]' <<<"$SSL_LIST")
upsert_by_name "pool" "$POOL_OBJ"
if [[ -n "$VSVIP_OBJ" ]]; then
  upsert_by_name "vsvip" "$VSVIP_OBJ"
fi
upsert_by_name "virtualservice" "$VS_OBJ"
