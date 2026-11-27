#!/usr/bin/env bash
# Real isolated git repos, fake GitHub/Go clients. Never writes a remote service.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/image-pr-test.XXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/repo/scripts"
cp "$root/scripts/update-runtime-image-pr.sh" "$root/scripts/runtime-image-update.jq" "$tmp/repo/scripts/"
export IMAGE_PR_TEST=$tmp GITHUB_REPOSITORY=apstndb/spanemuboost GITHUB_BASE_BRANCH=main
branch=automation/runtime-images-octo-sts
export IMAGE_PR_BRANCH=$branch
export PATH="$tmp/bin:$PATH"
cat > "$tmp/bin/go" <<'STUB'
#!/bin/sh
set -eu
if [ -f images_generated.go ] && grep -q "^//go:generate " images_generated.go; then
  echo "Untrusted generated Go file reached the generator" >&2
  exit 9
fi
jq -c . runtime-images.json > images_generated.go
STUB
cat > "$tmp/bin/gh" <<'STUB'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$IMAGE_PR_TEST/calls"
case "$1 $2" in
 'api users/apstndb-octo-sts%5Bbot%5D') printf '362053602\n' ;;
 'api --paginate')
   case "$3" in
     */pulls\?*)
       sha=$(git ls-remote origin "refs/heads/$IMAGE_PR_BRANCH" | cut -f1)
       jq --arg sha "$sha" 'map(if .state=="open" then .head.sha=$sha else . end)' "$IMAGE_PR_TEST/prs.json"
       ;;
     *) exit 7 ;;
   esac ;;
 'api repos/apstndb/spanemuboost/pulls'|'api repos/apstndb/spanemuboost/pulls/7')
   [ "${PR_FAIL:-0}" = 0 ] || exit 7
   [ "$3" = --method ]
   case "$2 $4" in
     */pulls\ POST|*/pulls/7\ PATCH) ;;
     *) exit 8 ;;
   esac
   body=
   for arg do
     case "$arg" in
       body=@*) body=${arg#body=@} ;;
     esac
   done
   test -n "$body"
   jq -n --rawfile body "$body" --arg sha "$(git rev-parse HEAD)" '[
     {number:7,state:"open",merged_at:null,body:$body,user:{id:362053602},
      base:{ref:"main"},head:{sha:$sha,repo:{full_name:"apstndb/spanemuboost"}}}
   ]' > "$IMAGE_PR_TEST/prs.json"
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
 if git show-ref --verify --quiet "refs/heads/$branch"; then git branch -D "$branch" >/dev/null; fi
 : > "$tmp/calls"
 bash scripts/update-runtime-image-pr.sh "$tmp/report.json" > "$tmp/out" 2> "$tmp/err" || { cat "$tmp/err" >&2; return 1; }
}
# An orphan push after PR creation failure is recovered without a new commit.
if PR_FAIL=1 run_update; then echo 'Expected PR API failure' >&2; exit 1; fi
orphan=$(git rev-parse "origin/$branch")
run_update
[[ $(git rev-parse "origin/$branch") == "$orphan" ]]
head1=$(git rev-parse "origin/$branch")
grep -q -- '--method POST' "$tmp/calls"
assert_no_call 'workflow run'
run_update
[[ $(git rev-parse "origin/$branch") == "$head1" ]]
assert_no_call 'workflow run|--method POST|--method PATCH'
# Even a matching marker cannot authorize adoption of a different PR author.
cp "$tmp/prs.json" "$tmp/owned-pr.json"
jq '.[0].user.id=123' "$tmp/owned-pr.json" > "$tmp/prs.json"
if run_update; then echo 'Adopted a PR created by another author' >&2; exit 1; fi
assert_no_call 'workflow run|--method POST|--method PATCH'
cp "$tmp/owned-pr.json" "$tmp/prs.json"
# A push followed by a failed edit is recovered without adding another commit.
jq '.images.DefaultEmulatorImage.image="e:3"' "$tmp/report.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/report.json"
if PR_FAIL=1 run_update; then echo 'Expected PR edit failure' >&2; exit 1; fi
before_retry=$(git rev-parse "origin/$branch")
run_update
[[ $(git rev-parse "origin/$branch") == "$before_retry" ]]
grep -q -- '--method PATCH' "$tmp/calls"
assert_no_call 'workflow run|--method POST'
head2=$(git rev-parse "origin/$branch")
git merge-base --is-ancestor "$head1" "$head2"
# If discovery returns to the old candidate after a failed edit, the live
# branch contents must be repaired even though the PR marker already matches.
jq '.images.DefaultEmulatorImage.image="e:3.1"' "$tmp/report.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/report.json"
if PR_FAIL=1 run_update; then echo 'Expected PR edit failure' >&2; exit 1; fi
pushed=$(git rev-parse "origin/$branch")
git show "origin/$branch:runtime-images.json" | jq -e '.DefaultEmulatorImage=="e:3.1"' >/dev/null
jq '.images.DefaultEmulatorImage.image="e:3"' "$tmp/report.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/report.json"
run_update
git merge-base --is-ancestor "$pushed" "origin/$branch"
git show "origin/$branch:runtime-images.json" | jq -e '.DefaultEmulatorImage=="e:3"' >/dev/null
# Closing a rejected candidate suppresses exactly that set.
jq '.[0].state="closed"' "$tmp/prs.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/prs.json"
run_update
assert_no_call 'workflow run|--method POST|--method PATCH'
# New candidate gets a new PR on the same additive branch history.
jq '.images.DefaultEmulatorImage.image="e:4"' "$tmp/report.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/report.json"
run_update
grep -q -- '--method POST' "$tmp/calls"
git merge-base --is-ancestor "$head2" "origin/$branch"
# Only trusted Go files may reach go generate, even with an allowed branch path.
git checkout -q "$branch"
printf '//go:generate unsafe-fixture\n' >> images_generated.go
git add images_generated.go
git commit -qm '[spanemuboost-runtime-images:v1] Untrusted generated input fixture'
git push -q origin HEAD
jq '.images.DefaultEmulatorImage.image="e:4.1"' "$tmp/report.json" > "$tmp/new.json"; mv "$tmp/new.json" "$tmp/report.json"
run_update
grep -q -- '--method PATCH' "$tmp/calls"
if git show "origin/$branch:images_generated.go" | grep -q 'unsafe-fixture'; then
  echo 'Untrusted directive survived regeneration' >&2
  exit 1
fi
# Even an unchanged candidate marker cannot authorize manual branch changes.
git checkout -q "$branch"
printf 'manual work\n' > human.txt;git add human.txt;git commit -qm human;git push -q origin HEAD
if run_update; then echo 'Accepted manual changes on matching fast path' >&2; exit 1; fi
assert_no_call 'workflow run|--method POST|--method PATCH'
# Historical closed PRs do not authorize a replacement branch, even if only
# generated/data files differ. Deliberate fixture reset affects only local remote.
git checkout -q main
git push -q origin --delete "$branch"
git checkout -q -b replacement main
jq '.DefaultEmulatorImage="e:9"' runtime-images.json > data.json;mv data.json runtime-images.json
go generate .;git add runtime-images.json images_generated.go;git commit -qm 'human replacement'
git push -q origin "HEAD:$branch"
jq '.[0].state="closed"' "$tmp/prs.json" > "$tmp/new.json";mv "$tmp/new.json" "$tmp/prs.json"
jq '.images.DefaultEmulatorImage.image="e:5"' "$tmp/report.json" > "$tmp/new.json";mv "$tmp/new.json" "$tmp/report.json"
if run_update; then echo 'Accepted replacement branch from historical PR ownership' >&2; exit 1; fi
assert_no_call 'workflow run|--method POST|--method PATCH'
printf 'Offline App PR lifecycle scenarios passed\n'
