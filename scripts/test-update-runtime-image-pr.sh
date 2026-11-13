#!/usr/bin/env bash
# Real isolated git repos, fake GitHub/Go clients. Never writes a remote service.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/image-pr-test.XXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/repo/scripts"
cp "$root/scripts/update-runtime-image-pr.sh" "$root/scripts/runtime-image-update.jq" "$tmp/repo/scripts/"
export IMAGE_PR_TEST=$tmp GITHUB_REPOSITORY=apstndb/spanemuboost GITHUB_BASE_BRANCH=main
export PATH="$tmp/bin:$PATH"
cat > "$tmp/bin/go" <<'STUB'
#!/bin/sh
set -eu
jq -c . runtime-images.json > images_generated.go
STUB
cat > "$tmp/bin/gh" <<'STUB'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$IMAGE_PR_TEST/calls"
case "$1 $2" in
 'api --paginate')
   case "$3" in
     */pulls\?*) cat "$IMAGE_PR_TEST/prs.json" ;;
     */runs\?*)
       workflow=${3#*/workflows/}; workflow=${workflow%%/runs*}
       if [ -f "$IMAGE_PR_TEST/runs-$workflow.json" ]; then cat "$IMAGE_PR_TEST/runs-$workflow.json"; else printf '{"workflow_runs":[]}\n'; fi ;;
     *) exit 7 ;;
   esac ;;
 'workflow run')
   if [ "${CI_FAIL_LINT:-0}" = 1 ] && [ "$3" = golangci-lint.yml ]; then exit 7; fi
   sha=$(git rev-parse origin/automation/runtime-images)
   workflow=$3
   jq -n --arg sha "$sha" '{workflow_runs:[{head_sha:$sha}]}' > "$IMAGE_PR_TEST/runs-$workflow.json" ;;
 'pr create'|'pr edit')
   if [ "${PR_FAIL:-0}" = 1 ]; then exit 7; fi
   while [ "$#" -gt 0 ]; do
    if [ "$1" = --body-file ]; then body=$2; break; fi
    shift
   done
   jq -n --rawfile body "$body" --arg sha "$(git rev-parse HEAD)" '[{number:7,state:"open",merged_at:null,body:$body,head:{sha:$sha,repo:{full_name:"apstndb/spanemuboost"}}}]' > "$IMAGE_PR_TEST/prs.json"
   printf 'https://github.com/apstndb/spanemuboost/pull/7\n' ;;
 *) exit 8 ;;
esac
STUB
chmod +x "$tmp/bin/go" "$tmp/bin/gh"
cd "$tmp/repo"
git init -q -b main
git config commit.gpgsign false
git config user.name fixture
git config user.email fixture@example.invalid
printf '{"DefaultEmulatorImage":"e:1","DefaultOmniImage":"o:1","LatestLTSOmniImage":"l:1"}\n' > runtime-images.json
go generate .
git add scripts runtime-images.json images_generated.go
git commit -qm initial
git init -q --bare "$tmp/remote"
git remote add origin "$tmp/remote"
git push -q -u origin main
GITHUB_SHA=$(git rev-parse main)
export GITHUB_SHA
printf '[]\n' > "$tmp/prs.json"
cat > "$tmp/report.json" <<'JSON'
{"images":{"DefaultEmulatorImage":{"current_image":"e:1","image":"e:2","update_available":true},"DefaultOmniImage":{"current_image":"o:1","image":"o:1","update_available":false},"LatestLTSOmniImage":{"current_image":"l:1","image":"l:1","update_available":false}}}
JSON
assert_no_call() {
 if grep -Eq "$1" "$tmp/calls"; then echo "Unexpected API call: $1" >&2; exit 1; fi
}
run_update() {
 git checkout -q main
 if git show-ref --verify --quiet refs/heads/automation/runtime-images; then git branch -D automation/runtime-images >/dev/null; fi
 : > "$tmp/calls"
 bash scripts/update-runtime-image-pr.sh "$tmp/report.json" > "$tmp/out" 2> "$tmp/err" || { cat "$tmp/err" >&2; return 1; }
}
# An orphan push after PR creation failure is recovered without a new commit.
if PR_FAIL=1 run_update; then echo 'Expected PR API failure' >&2; exit 1; fi
orphan=$(git rev-parse origin/automation/runtime-images)
run_update
[[ $(git rev-parse origin/automation/runtime-images) == "$orphan" ]]
head1=$(git rev-parse origin/automation/runtime-images)
grep -q 'pr create' "$tmp/calls"
# New PR dispatches both CI workflows; each endpoint is independently checked.
[[ $(grep -c 'workflow run' "$tmp/calls") == 2 ]]
# Existing CI may be on an earlier API page; no duplicate dispatch is allowed.
for workflow in go.yml golangci-lint.yml; do printf '{"workflow_runs":[]}\n' >> "$tmp/runs-$workflow.json"; done
run_update
[[ $(git rev-parse origin/automation/runtime-images) == "$head1" ]]
assert_no_call 'pr create|pr edit|workflow run'
# Different candidates append a commit and edit the existing PR, never force push.
jq '.images.DefaultEmulatorImage.image="e:3"' "$tmp/report.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/report.json"
rm -f "$tmp"/runs-*.json
# PR update succeeds but a partial CI dispatch fails; retry dispatches only missing CI.
if CI_FAIL_LINT=1 run_update; then echo 'Expected dispatch failure' >&2; exit 1; fi
before_retry=$(git rev-parse origin/automation/runtime-images)
run_update
[[ $(git rev-parse origin/automation/runtime-images) == "$before_retry" ]]
[[ $(grep -c 'workflow run' "$tmp/calls") == 1 ]]
head2=$(git rev-parse origin/automation/runtime-images)
git merge-base --is-ancestor "$head1" "$head2"
assert_no_call 'pr edit'
assert_no_call 'pr create'
# Closing a rejected candidate suppresses exactly that set.
jq '.[0].state="closed"' "$tmp/prs.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/prs.json"
run_update
assert_no_call 'pr create|pr edit|workflow run'
# New candidate gets a new PR on the same additive branch history.
jq '.images.DefaultEmulatorImage.image="e:4"' "$tmp/report.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/report.json"
run_update
grep -q 'pr create' "$tmp/calls"
git merge-base --is-ancestor "$head2" origin/automation/runtime-images
# Non-bot work on the branch is never overwritten.
git checkout -q automation/runtime-images
printf 'manual work\n' > human.txt;git add human.txt;git commit -qm human;git push -q origin HEAD
jq '.images.DefaultEmulatorImage.image="e:5"' "$tmp/report.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/report.json"
if run_update; then echo 'Accepted unrelated branch changes' >&2; exit 1; fi
printf '8 offline PR lifecycle scenarios passed\n'
