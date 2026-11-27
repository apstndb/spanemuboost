#!/usr/bin/env bash
# Default-branch updater. Uses additive commits/merges, never force-pushes or merges PRs.
set -Eeuo pipefail
trap 'printf "Runtime image PR update failed (line %s)\n" "$LINENO" >&2; exit 2' ERR
[[ $# == 1 ]] || { echo 'Expected discovery JSON path' >&2; exit 2; }
report=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
repo=${GITHUB_REPOSITORY:?}
[[ "$repo" =~ ^apstndb/[A-Za-z0-9_.-]+$ ]] || exit 2
branch=automation/runtime-images-octo-sts
bot='apstndb-octo-sts[bot]'
base=${GITHUB_BASE_BRANCH:-main}
marker='<!-- spanemuboost-runtime-image-pr:v1 -->'
scratch=$(mktemp -d "${TMPDIR:-/tmp}/runtime-image-pr.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
jq -e -L "$root/scripts" 'include "runtime-image-update"; validate_report' "$report" > "$scratch/report.json"
if ! jq -e '.images | any(.update_available)' "$scratch/report.json" >/dev/null; then echo 'No updates'; exit 0; fi
key=$(jq -cr '.images|to_entries|map(select(.value.update_available)|{name:.key,current:.value.current_image,available:.value.image})|sort_by(.name)|tojson|@base64' "$scratch/report.json")
versions="<!-- runtime-image-candidates:$key -->"
bot_path=$(jq -nr --arg bot "$bot" '$bot|@uri')
bot_id=$(gh api "users/$bot_path" --jq '.id')
[[ "$bot_id" =~ ^[1-9][0-9]*$ ]]
gh api --paginate "repos/$repo/pulls?state=all&head=${repo%%/*}:$branch&per_page=100" > "$scratch/pages.json"
jq -s --arg repo "$repo" 'add|map(select(.head.repo.full_name==$repo))' "$scratch/pages.json" > "$scratch/all.json"
# A PR on the reserved branch must have the App author, marker, and base.
jq -e --arg marker "$marker" --argjson bot "$bot_id" --arg base "$base" '
 def managed: .user.id == $bot and .base.ref == $base and ((.body//"")|contains($marker));
 if all(.[]; .state!="open" or managed) then map(select(managed))
 else error("Unexpected open PR on the managed branch") end
' "$scratch/all.json" > "$scratch/prs.json"
if jq -e --arg v "$versions" 'any(.[]; .state=="closed" and .merged_at==null and ((.body//"")|contains($v)))' "$scratch/prs.json" >/dev/null; then
 echo 'Candidate set was dismissed'; exit 0
fi
matching=$(jq -r --arg v "$versions" '[.[]|select(.state=="open" and ((.body//"")|contains($v)))][0].head.sha//empty' "$scratch/prs.json")
[[ -z $(git status --porcelain) ]] || { echo 'Checkout must be clean' >&2; exit 2; }
git fetch origin "$base"
[[ $(git rev-parse HEAD) == "${GITHUB_SHA:?}" && $(git rev-parse "origin/$base") == "$GITHUB_SHA" ]] || { echo 'Default branch moved; retry discovery' >&2; exit 2; }
jq -e --slurpfile current runtime-images.json '.images|to_entries|all(.value.current_image==$current[0][.key])' "$scratch/report.json" >/dev/null
if git ls-remote --exit-code --heads origin "$branch" > "$scratch/ref"; then ref_rc=0; else ref_rc=$?; fi
if [[ "$ref_rc" == 0 ]]; then
 git fetch origin "$branch"
 # Only the two bot-owned files may differ from the common ancestor.
 ancestor=$(git merge-base "origin/$base" "origin/$branch")
 git diff --name-only "$ancestor" "origin/$branch" > "$scratch/paths"
 if grep -Ev '^(runtime-images.json|images_generated.go)$' "$scratch/paths"; then echo 'Unexpected branch changes' >&2; exit 2; fi
 # Historical PRs do not prove ownership of a deleted/recreated branch.
 # A manual tip stops automation, including the unchanged-candidate fast path.
 git log -1 --format=%B "origin/$branch" | grep -F '[spanemuboost-runtime-images:v1]' >/dev/null
else
 [[ "$ref_rc" == 2 ]] || exit 2
fi
if [[ -n "$matching" ]]; then
 [[ "$ref_rc" == 0 && $(git rev-parse "origin/$branch") == "$matching" ]] || { echo 'PR branch moved; retry discovery' >&2; exit 2; }
 # A successful push and failed PR edit can leave the candidate marker stale.
 if git show "origin/$branch:runtime-images.json" |
    jq -e --slurpfile report "$scratch/report.json" '. == ($report[0].images|map_values(.image))' >/dev/null; then
  echo 'Existing PR covers these candidates'; exit 0
 fi
fi
if [[ "$ref_rc" == 0 ]]; then git checkout -b "$branch" "origin/$branch"; else git checkout -b "$branch" "origin/$base"; fi
git config user.name "$bot"
git config user.email "$bot_id+$bot@users.noreply.github.com"
# Conflicts are limited to the validated bot-owned files and regenerated below.
git -c commit.gpgsign=false merge --no-edit -m '[spanemuboost-runtime-images:v1] Merge current default branch' -X theirs "origin/$base"
# The generator scans Go files for directives. Restore both inputs from the
# trusted base before processing the report, including their file types.
git restore --source "origin/$base" -- runtime-images.json images_generated.go
jq '.images|map_values(.image)' "$scratch/report.json" > runtime-images.json
go generate .
git diff --name-only > "$scratch/paths"
if grep -Ev '^(runtime-images.json|images_generated.go)$' "$scratch/paths"; then exit 2; fi
git add runtime-images.json images_generated.go
if ! git diff --cached --quiet; then git -c commit.gpgsign=false commit -m '[spanemuboost-runtime-images:v1] Update pinned runtime images'; fi
git push origin "HEAD:refs/heads/$branch"
jq -r -L "$root/scripts" --arg marker "$marker" --arg versions "$versions" \
 'include "runtime-image-update"; pr_body($marker;$versions)' "$scratch/report.json" > "$scratch/body.md"
number=$(jq -r '[.[]|select(.state=="open")][0].number//empty' "$scratch/prs.json")
if [[ -n "$number" ]]; then
 gh api "repos/$repo/pulls/$number" --method PATCH -f title='Update pinned runtime images' \
  -F body=@"$scratch/body.md" --jq '.html_url'
else
 gh api "repos/$repo/pulls" --method POST -f base="$base" -f head="$branch" \
  -f title='Update pinned runtime images' -F body=@"$scratch/body.md" --jq '.html_url'
fi
