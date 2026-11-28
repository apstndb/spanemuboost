#!/usr/bin/env bash
# With no argument, compute eligibility. With a matching plan, reserve auto-merge.
set -euo pipefail

skip() {
  printf 'Skipping auto-merge: %s\n' "$1" >&2
  exit 0
}

[[ $# -le 1 ]] || exit 2
repo=${GITHUB_REPOSITORY:?}
repo_id=${GITHUB_REPOSITORY_ID:?}
[[ "$repo" == apstndb/spanemuboost && "$repo_id" == 902526165 ]] || exit 2
branch=automation/runtime-images-octo-sts
bot_id=362053602
marker='<!-- spanemuboost-runtime-image-pr:v1 -->'
tip_marker='[spanemuboost-runtime-images:v1]'

context=runtime-image-merge-policy
target_url="$GITHUB_SERVER_URL/$repo/actions/runs/$GITHUB_RUN_ID"

publish_status() {
  gh api "repos/$repo/statuses/$1" --method POST -f context="$context" -f state=success \
    -f description="$2" -f target_url="$target_url" --silent
}

if [[ -n "${EVENT_PR:-}" ]]; then
  number=$EVENT_PR
else
  candidates=$(gh api "repos/$repo/pulls" --method GET --paginate --slurp \
    -f state=open -f base=main -f head="${repo%%/*}:$branch")
  number=$(jq -er 'add | select(length==1) | .[0].number | select(type=="number")' <<<"$candidates") ||
    skip 'expected one open PR on the managed branch'
fi
[[ "$number" =~ ^[1-9][0-9]*$ ]] || exit 2
pr_endpoint="repos/$repo/pulls/$number"
pr=$(gh api "$pr_endpoint")
head=$(jq -er --argjson repo_id "$repo_id" '
  select(.state=="open" and .base.repo.id==$repo_id and .base.ref=="main")
  | .head.sha | select(test("^[a-f0-9]{40}$"))
' <<<"$pr") || skip 'no open PR targeting main'
if [[ -n "${EVENT_HEAD:-}" && "$EVENT_HEAD" != "$head" ]]; then
  skip 'event is for a different head'
fi

if ! jq -e --argjson repo_id "$repo_id" --arg branch "$branch" '
  .head.repo.id==$repo_id and .head.ref==$branch
' <<<"$pr" >/dev/null; then
  # Ordinary PRs still satisfy the four independently required CI checks.
  [[ $# == 0 && "$GITHUB_EVENT_NAME" != workflow_run ]] || skip 'ordinary PR'
  # Commit statuses are shared by SHA. Never grant an ordinary-PR pass to
  # the same commit as an App candidate opened under another branch name.
  commit=$(gh api "repos/$repo/git/commits/$head")
  if ! jq -e --arg head "$head" --arg marker "$tip_marker" '
    .sha==$head and (.message | type=="string" and (contains($marker)|not))
  ' <<<"$commit" >/dev/null; then
    skip 'commit is marked as a managed image update'
  fi
  publish_status "$head" 'Ordinary PR; image-specific policy does not apply'
  exit 0
fi
# Managed PRs get their gate only from checks on CI completion or manual recheck.
if [[ "$GITHUB_EVENT_NAME" == pull_request_target ]]; then
  skip 'managed PR awaits CI completion'
fi
plan=$(jq -ce --argjson repo_id "$repo_id" --argjson bot_id "$bot_id" --arg branch "$branch" --arg marker "$marker" '
  select(.state=="open" and .draft==false and .mergeable==true
    and .user.id==$bot_id and .user.type=="Bot" and ((.body//"") | contains($marker))
    and .base.repo.id==$repo_id and .head.repo.id==$repo_id
    and .base.ref=="main" and .head.ref==$branch and .changed_files==2)
  | select(.head.sha | test("^[a-f0-9]{40}$"))
  | select(.base.sha | test("^[a-f0-9]{40}$"))
  | select(.node_id | type=="string" and length>0)
  | {number,node:.node_id,head:.head.sha,base:.base.sha}
' <<<"$pr") || skip 'PR does not meet the identity or file-count requirements'

if [[ $# == 1 && "$plan" != "$1" ]]; then
  skip 'PR head or base changed'
fi
base=$(jq -r .base <<<"$plan")

if ! gh api "repos/$repo/git/commits/$head" |
  jq -e --arg head "$head" --arg marker "$tip_marker"     '.sha==$head and (.message|contains($marker))' >/dev/null; then
  skip 'head lacks the managed commit marker'
fi

# Compare immutable commits. An up-to-date head also prevents accepting a
# pass obtained before main advanced.
comparison=$(gh api "repos/$repo/compare/$base...$head")
if ! jq -e --arg base "$base" '
  .status=="ahead" and .merge_base_commit.sha==$base
  and (.files | map(.filename) | sort)==["images_generated.go","runtime-images.json"]
  and all(.files[]; .status=="modified")
' <<<"$comparison" >/dev/null; then
  skip 'head is behind main or changes files outside the image update'
fi

check_workflow() {
  local workflow=$1 expected_jobs=$2 runs run id attempt jobs
  runs=$(gh api "repos/$repo/actions/workflows/$workflow/runs" --method GET --paginate --slurp \
    -f event=pull_request -f head_sha="$head" -f branch="$branch" -f per_page=100) || return 2
  # Check the newest relevant run even if it is pending or failed. Both
  # workflows target main for PRs. The API can omit PR associations, but
  # an explicit association must identify this PR.
  run=$(jq -ce --arg head "$head" --arg branch "$branch" --argjson repo_id "$repo_id" \
    --argjson bot_id "$bot_id" --argjson pr "$number" '
    [.[].workflow_runs[] | select(.event=="pull_request" and .head_sha==$head and .head_branch==$branch
      and .head_repository.id==$repo_id and
        ((.pull_requests | length)==0 or any(.pull_requests[]; .number==$pr and .base.ref=="main")))]
    | max_by(.id) | select(.actor.id==$bot_id and .status=="completed" and .conclusion=="success")
  ' <<<"$runs") || return 1
  id=$(jq -er '.id | select(type=="number")' <<<"$run") || return 2
  attempt=$(jq -er '.run_attempt | select(type=="number")' <<<"$run") || return 2
  jobs=$(gh api "repos/$repo/actions/runs/$id/jobs" --method GET --paginate --slurp -f filter=all) || return 2
  # A failed-jobs-only rerun can retain successes from earlier attempts.
  jq -e --argjson names "$expected_jobs" --arg head "$head" --argjson attempt "$attempt" '
    [.[].jobs[]] | group_by(.name) | map(max_by(.run_attempt))
    | (map(.name)|sort)==$names and ([.[].run_attempt] | max)==$attempt
    and all(.[]; .head_sha==$head and .status=="completed" and .conclusion=="success")
  ' <<<"$jobs" >/dev/null
}

for workflow in go.yml golangci-lint.yml; do
  if [[ "$workflow" == go.yml ]]; then
    jobs='["build-and-test","omni-smoke","podman-smoke"]'
  else
    jobs='["lint"]'
  fi
  if check_workflow "$workflow" "$jobs"; then
    continue
  else
    result=$?
    [[ "$result" == 1 ]] || exit "$result"
    skip "$workflow has not passed for this head"
  fi
done

if [[ $# == 0 ]]; then
  printf '%s\n' "$plan"
  exit 0
fi

# Keep the gate closed unless the server requires it and the four CI checks.
gh api "repos/$repo/branches/main" |
  jq -e --arg context "$context" '
    .protected and ((.protection.required_status_checks.contexts // [])
    | contains(["build-and-test","lint","omni-smoke","podman-smoke",$context]))
  ' >/dev/null
# The REST repository response omits merge settings for read-only callers.
# shellcheck disable=SC2016
gh api graphql -f query='
  query($owner:String!,$name:String!) {
    repository(owner:$owner,name:$name) {
      autoMergeAllowed squashMergeAllowed mergeCommitAllowed rebaseMergeAllowed
    }
  }
' -f owner="${repo%%/*}" -f name="${repo#*/}" |
  jq -e '.data.repository | .autoMergeAllowed and .squashMergeAllowed
    and .mergeCommitAllowed==false and .rebaseMergeAllowed==false' >/dev/null

# Check the minted token's scope before making the GraphQL reservation.
GH_TOKEN=${RESERVATION_TOKEN:?} gh api installation/repositories --paginate --slurp |
  jq -e --argjson repo_id "$repo_id" '[.[].repositories[].id]==[$repo_id]' >/dev/null
pr=$(gh api "$pr_endpoint")
jq -e --arg head "$head" --arg base "$base" '
  .state=="open" and .draft==false and .mergeable==true and .base.ref=="main"
    and .head.sha==$head and .base.sha==$base and (.auto_merge.merge_method // "squash")=="squash"
' <<<"$pr" >/dev/null
if ! jq -e '.auto_merge.merge_method=="squash"' <<<"$pr" >/dev/null; then
  node=$(jq -r .node <<<"$plan")
  # GraphQL variables are bound by gh below.
  # shellcheck disable=SC2016
  GH_TOKEN=$RESERVATION_TOKEN gh api graphql -f query='
    mutation($id:ID!,$head:GitObjectID!) {
      enablePullRequestAutoMerge(input:{pullRequestId:$id,expectedHeadOid:$head,mergeMethod:SQUASH}) {
        pullRequest { headRefOid autoMergeRequest { mergeMethod } }
      }
    }
  ' -f id="$node" -f head="$head" | jq -e --arg head "$head" '
    .data.enablePullRequestAutoMerge.pullRequest
    | .headRefOid==$head and .autoMergeRequest.mergeMethod=="SQUASH"
  ' >/dev/null
fi
# GitHub applies its native required checks again before merging.
publish_status "$head" 'Exact image diff and all four CI jobs passed'
printf 'Native squash auto-merge enabled for PR #%s at %s\n' "$number" "$head"
