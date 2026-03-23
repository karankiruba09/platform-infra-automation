#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  list-harbor-helm-charts.sh [harbor-url] [username] [password] [output-csv]

Example:
  HARBOR_URL=https://harbor.example.com HARBOR_USER=admin HARBOR_PASSWORD='secret' \
    list-harbor-helm-charts.sh
EOF
}

if [ "$#" -gt 4 ]; then
  usage >&2
  exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

harbor_url="${1:-${HARBOR_URL:-}}"
harbor_user="${2:-${HARBOR_USER:-}}"
harbor_password="${3:-${HARBOR_PASSWORD:-}}"
output_csv="${4:-${OUTPUT_CSV:-${script_dir}/output/harbor-helm-charts.csv}}"

if [ -z "$harbor_url" ] || [ -z "$harbor_user" ] || [ -z "$harbor_password" ]; then
  usage >&2
  exit 1
fi

harbor_url="${harbor_url%/}"
case "$harbor_url" in
  http://*|https://*) ;;
  *) harbor_url="https://${harbor_url}" ;;
esac
output_total="${output_csv%.csv}.total.txt"

page_size="${PAGE_SIZE:-100}"
parallelism="${PARALLELISM:-32}"
auth="${harbor_user}:${harbor_password}"
curl_args=(--path-as-is -fsS -u "$auth")
rows_written=0
tmp_dir="$(mktemp -d)"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

if [ "${HARBOR_INSECURE:-false}" = "true" ]; then
  curl_args+=(-k)
fi

fetch_json() {
  local url="$1"
  curl "${curl_args[@]}" "$url" 2>/dev/null
}

printf 'project,repository,version\n' >"$output_csv"
printf 'total_oci_helm_chart_versions,0\n' >"$output_total"
printf 'Listing OCI Helm charts from %s\n' "$harbor_url" >&2
printf 'Using parallelism: %s\n' "$parallelism" >&2

project_page=1

while :; do
  project_payload="$(fetch_json "${harbor_url}/api/v2.0/projects?page=${project_page}&page_size=${page_size}")"

  project_count="$(jq 'length' <<<"$project_payload")"
  [ "$project_count" -eq 0 ] && break

  while IFS=$'\t' read -r project_name repository_count; do
    [ -z "$project_name" ] && continue
    [ "${repository_count:-0}" -eq 0 ] && continue
    printf 'Scanning project: %s (%s repositories)\n' "$project_name" "$repository_count" >&2
    repo_page=1
    project_tmp="${tmp_dir}/${project_name}.csv"
    : >"$project_tmp"

    while :; do
      repo_payload="$(fetch_json "${harbor_url}/api/v2.0/projects/${project_name}/repositories?page=${repo_page}&page_size=${page_size}")"
      repo_count="$(jq 'length' <<<"$repo_payload")"
      [ "$repo_count" -eq 0 ] && break

      jq -r '.[].name' <<<"$repo_payload" | \
      xargs -r -P "$parallelism" -I {} /usr/bin/bash -lc '
        set -euo pipefail
        repo="$1"
        project_name="$2"
        harbor_url="$3"
        page_size="$4"
        project_tmp="$5"
        shift 5
        curl_args=("$@")

        repo_path="${repo#${project_name}/}"
        repo_encoded="$(jq -nr --arg v "$repo_path" '"'"'$v|@uri|@uri'"'"')"
        artifact_page=1

        while :; do
          if ! artifact_payload="$(curl "${curl_args[@]}" \
            "${harbor_url}/api/v2.0/projects/${project_name}/repositories/${repo_encoded}/artifacts?page=${artifact_page}&page_size=${page_size}&with_tag=true" \
            2>/dev/null)"; then
            break
          fi

          artifact_count="$(jq '"'"'length'"'"' <<<"$artifact_payload")"
          [ "$artifact_count" -eq 0 ] && break

          jq -r --arg project "$project_name" --arg repo "$repo" '"'"'
            .[] |
            select(
              .type == "CHART" or
              .artifact_type == "application/vnd.cncf.helm.config.v1+json" or
              .media_type == "application/vnd.cncf.helm.config.v1+json"
            ) |
            if (.tags | length) > 0 then
              .tags[] | [$project, $repo, .name]
            else
              [$project, $repo, "<untagged>"]
            end |
            @csv
          '"'"' <<<"$artifact_payload" >>"$project_tmp"

          artifact_page=$((artifact_page + 1))
        done
      ' _ {} "$project_name" "$harbor_url" "$page_size" "$project_tmp" "${curl_args[@]}"

      repo_page=$((repo_page + 1))
    done

    if [ -s "$project_tmp" ]; then
      cat "$project_tmp" >>"$output_csv"
      project_rows="$(wc -l <"$project_tmp")"
      rows_written=$((rows_written + project_rows))
      printf 'Found %s OCI Helm chart version entries in %s\n' "$project_rows" "$project_name" >&2
    else
      printf 'Found 0 OCI Helm chart version entries in %s\n' "$project_name" >&2
    fi
  done < <(jq -r '.[] | [.name, (.repo_count // 0)] | @tsv' <<<"$project_payload")

  project_page=$((project_page + 1))
done

cat "$output_csv"
printf 'total_oci_helm_chart_versions,%s\n' "$rows_written" >"$output_total"

if [ "$rows_written" -eq 0 ]; then
  printf 'No OCI Helm charts found. CSV saved to %s\n' "$output_csv" >&2
else
  printf 'Found %s OCI Helm chart version entries. CSV saved to %s\n' "$rows_written" "$output_csv" >&2
fi

printf 'Total file saved to %s\n' "$output_total" >&2
