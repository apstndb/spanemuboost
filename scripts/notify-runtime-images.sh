#!/usr/bin/env bash
# Publish only verified discovery drift. Intended for serialized default-branch
# schedule/workflow_dispatch runs. No runtime startup, upgrades or releases.
set -Eeuo pipefail
trap 'printf "Image notification failed (line %s)\n" "$LINENO" >&2; exit 2' ERR
if [[ $# != 1 ]]; then printf 'Usage: notify-runtime-images.sh REPORT.json\n' >&2; exit 2; fi
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
repo=${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}
if [[ ! "$repo" =~ ^apstndb/[A-Za-z0-9_.-]+$ ]]; then
  printf 'Expected an apstndb repository\n' >&2; exit 2
fi
scratch=$(mktemp -d "${TMPDIR:-/tmp}/runtime-image-issue.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
jq -e -L "$root/scripts" 'include "runtime-image-notification"; validate_report' "$1" > "$scratch/report.json"
if ! jq -e '.images | any(.update_available)' "$scratch/report.json" >/dev/null; then
  printf 'No runtime image updates\n'; exit 0
fi
# Base64 encodes the canonical version-reference set; no extra hash dependency.
# checked_at, metadata changes and run URLs deliberately do not affect dedup.
key=$(jq -crS '.images | to_entries | map(select(.value.update_available) | {name:.key,current:.value.current_image,available:.value.image}) | sort_by(.name) | tojson | @base64' "$scratch/report.json")
marker='<!-- spanemuboost-runtime-image-updates:v1 -->'
version_marker="<!-- runtime-image-candidates:$key -->"
run_url="${GITHUB_SERVER_URL:-https://github.com}/$repo/actions/runs/${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
jq -r -L "$root/scripts" --arg marker "$marker" --arg versions "$version_marker" --arg run "$run_url" \
  'include "runtime-image-notification"; issue_body($marker;$versions;$run)' "$scratch/report.json" > "$scratch/body.md"
# Use all pages and exclude PRs. Closed issues remember intentionally dismissed
# version sets; identical candidates do not create daily replacement issues.
gh api --paginate "repos/$repo/issues?state=all&per_page=100" > "$scratch/pages.json"
jq -s --arg marker "$marker" 'add | map(select(.pull_request == null and ((.body // "") | contains($marker)))) | sort_by(.number)' "$scratch/pages.json" > "$scratch/issues.json"
if jq -e --arg versions "$version_marker" 'any(.[]; .state == "closed" and ((.body // "") | contains($versions)))' "$scratch/issues.json" >/dev/null; then
  printf 'These candidates were already dismissed in a closed issue\n'; exit 0
fi
if jq -e --arg versions "$version_marker" 'any(.[]; .state == "open" and ((.body // "") | contains($versions)))' "$scratch/issues.json" >/dev/null; then
  printf 'An open tracking issue already covers these candidates\n'; exit 0
fi
open_number=$(jq -r '[.[] | select(.state == "open")][0].number // empty' "$scratch/issues.json")
if [[ -n "$open_number" ]]; then
  method=PATCH
  endpoint="repos/$repo/issues/$open_number"
else
  method=POST
  endpoint="repos/$repo/issues"
fi
jq -n --rawfile body "$scratch/body.md" '{title:"Runtime image updates available",body:$body}' > "$scratch/issue.json"
gh api --method "$method" "$endpoint" --input "$scratch/issue.json" --jq .html_url
