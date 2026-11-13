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
branch=automation/runtime-images
base=${GITHUB_BASE_BRANCH:-main}
marker='<!-- spanemuboost-runtime-image-pr:v1 -->'
scratch=$(mktemp -d "${TMPDIR:-/tmp}/runtime-image-pr.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
jq -e -L "$root/scripts" 'include "runtime-image-update"; validate_report' "$report" > "$scratch/report.json"
if ! jq -e '.images | any(.update_available)' "$scratch/report.json" >/dev/null; then echo 'No updates'; exit 0; fi
key=$(jq -cr '.images|to_entries|map(select(.value.update_available)|{name:.key,current:.value.current_image,available:.value.image})|sort_by(.name)|tojson|@base64' "$scratch/report.json")
versions="<!-- runtime-image-candidates:$key -->"
gh api --paginate "repos/$repo/pulls?state=all&head=${repo%%/*}:$branch&per_page=100" > "$scratch/pages.json"
jq -s --arg repo "$repo" 'add|map(select(.head.repo.full_name==$repo))' "$scratch/pages.json" > "$scratch/all.json"
# Never adopt a human PR merely because it uses the reserved branch name.
jq -e --arg marker "$marker" 'all(.[]; .state!="open" or ((.body//"")|contains($marker)))' "$scratch/all.json" >/dev/null
jq --arg marker "$marker" 'map(select((.body//"")|contains($marker)))' "$scratch/all.json" > "$scratch/prs.json"
if jq -e --arg v "$versions" 'any(.[]; .state=="closed" and .merged_at==null and ((.body//"")|contains($v)))' "$scratch/prs.json" >/dev/null; then
 echo 'Candidate set was dismissed'; exit 0
fi
ensure_ci() {
 local sha=$1 workflow
 for workflow in go.yml golangci-lint.yml; do
  gh api --paginate "repos/$repo/actions/workflows/$workflow/runs?event=workflow_dispatch&branch=$branch&per_page=100" > "$scratch/runs.json"
  if ! jq -se --arg sha "$sha" '[.[].workflow_runs[]]|any(.head_sha==$sha)' "$scratch/runs.json" >/dev/null; then
   gh workflow run "$workflow" --repo "$repo" --ref "$branch"
  fi
 done
}
matching=$(jq -r --arg v "$versions" '[.[]|select(.state=="open" and ((.body//"")|contains($v)))][0].head.sha//empty' "$scratch/prs.json")
if [[ -n "$matching" ]]; then ensure_ci "$matching"; echo 'Existing PR covers these candidates'; exit 0; fi
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
 if ! jq -e 'length>0' "$scratch/prs.json" >/dev/null; then
  # Recover a push that succeeded before PR creation failed, but not an unowned branch.
  git log -1 --format=%B "origin/$branch" | grep -F '[spanemuboost-runtime-images:v1]' >/dev/null
 fi
 git checkout -b "$branch" "origin/$branch"
else
 [[ "$ref_rc" == 2 ]] || exit 2
 git checkout -b "$branch" "origin/$base"
fi
git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
# Conflicts are limited to the validated bot-owned files and regenerated below.
git -c commit.gpgsign=false merge --no-edit -m '[spanemuboost-runtime-images:v1] Merge current default branch' -X theirs "origin/$base"
jq '.images|map_values(.image)' "$scratch/report.json" > runtime-images.json
go generate .
git diff --name-only > "$scratch/paths"
if grep -Ev '^(runtime-images.json|images_generated.go)$' "$scratch/paths"; then exit 2; fi
git add runtime-images.json images_generated.go
if ! git diff --cached --quiet; then git -c commit.gpgsign=false commit -m '[spanemuboost-runtime-images:v1] Update pinned runtime images'; fi
git push origin "HEAD:refs/heads/$branch"
sha=$(git rev-parse HEAD)
jq -r -L "$root/scripts" --arg marker "$marker" --arg versions "$versions" \
 'include "runtime-image-update"; pr_body($marker;$versions)' "$scratch/report.json" > "$scratch/body.md"
number=$(jq -r '[.[]|select(.state=="open")][0].number//empty' "$scratch/prs.json")
if [[ -n "$number" ]]; then
 gh pr edit "$number" --repo "$repo" --title 'Update pinned runtime images' --body-file "$scratch/body.md"
else
 gh pr create --repo "$repo" --base "$base" --head "$branch" --title 'Update pinned runtime images' --body-file "$scratch/body.md"
fi
ensure_ci "$sha"
