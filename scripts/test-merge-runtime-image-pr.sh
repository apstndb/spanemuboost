#!/usr/bin/env bash
# Check the merge gate with a fake GitHub CLI. No live API requests.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
fixture=$(mktemp -d "${TMPDIR:-/tmp}/image-merge-test.XXXXXX")
trap 'rm -rf -- "$fixture"' EXIT
export MERGE_FIXTURE=$fixture PATH="$fixture:$PATH"
export GITHUB_REPOSITORY=apstndb/spanemuboost GITHUB_REPOSITORY_ID=902526165
export GH_TOKEN=fixture-status RESERVATION_TOKEN=fixture-app
export GITHUB_EVENT_NAME=workflow_run GITHUB_SERVER_URL=https://github.com GITHUB_RUN_ID=42
export HEAD_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export BASE_SHA=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
export MANAGED_BRANCH=automation/runtime-images-octo-sts
export SCENARIO=pass
cat >"$fixture/gh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == api ]]
case "$2" in
  repos/apstndb/spanemuboost/pulls)
    if [[ "$SCENARIO" == no-pr ]]; then echo '[[]]'; else echo '[[{"number":123}]]'; fi ;;
  repos/apstndb/spanemuboost/pulls/123)
    jq -n --arg head "$HEAD_SHA" --arg base "$BASE_SHA" --arg branch "$MANAGED_BRANCH" \
      --arg scenario "$SCENARIO" '
      def repo: {id:902526165};
      {number:123,node_id:"fixture-node",state:"open",draft:($scenario=="draft"),mergeable:true,changed_files:2,
        user:{type:"Bot",id:(if $scenario=="other-bot" then 123 else 362053602 end)},
        body:"<!-- spanemuboost-runtime-image-pr:v1 -->",
        head:{sha:$head,ref:$branch,repo:repo},base:{sha:$base,ref:"main",repo:repo}}
      | if $scenario=="fork" then .head.repo.id=1
        elif ($scenario=="ordinary" or $scenario=="copied-tip")
          then .head.ref="topic" | .user={id:123,type:"User"}
        elif $scenario=="armed" then .auto_merge={merge_method:"squash"}
        else . end
    ' ;;
  repos/apstndb/spanemuboost/git/commits/*)
    jq -n --arg head "$HEAD_SHA" --arg scenario "$SCENARIO" '
      {sha:$head,message:(if $scenario=="ordinary" or $scenario=="unmarked" then "Topic"
       else "[spanemuboost-runtime-images:v1] Fixture" end)}
    ' ;;
  repos/apstndb/spanemuboost/branches/main)
    if [[ "$SCENARIO" == no-required-gate ]]; then
      echo '{"protected":true,"protection":{"required_status_checks":{"contexts":[]}}}'
    else
      echo '{"protected":true,"protection":{"required_status_checks":{"contexts":["lint","omni-smoke","runtime-image-merge-policy","podman-smoke","build-and-test"]}}}'
    fi ;;
  repos/apstndb/spanemuboost)
    echo '{"allow_auto_merge":true,"allow_squash_merge":true,"allow_merge_commit":false,"allow_rebase_merge":false}' ;;
  repos/apstndb/spanemuboost/compare/*)
    [[ "$2" == "repos/apstndb/spanemuboost/compare/$BASE_SHA...$HEAD_SHA" ]]
    jq -n --arg base "$BASE_SHA" --arg scenario "$SCENARIO" '
      {status:"ahead",merge_base_commit:{sha:$base},
        files:[{filename:"images_generated.go",status:"modified"},{filename:"runtime-images.json",status:"modified"}]}
      | if $scenario=="rename" then .files[0].status="renamed"
        elif $scenario=="extra-path" then .files[0].filename="README.md"
        elif $scenario=="stale-base" then .merge_base_commit.sha="old"
        else . end
    ' ;;
  repos/apstndb/spanemuboost/actions/workflows/*/runs)
    workflow=${2##*/workflows/}; workflow=${workflow%/runs}
    jq -n --arg head "$HEAD_SHA" --arg branch "$MANAGED_BRANCH" --arg scenario "$SCENARIO" --arg workflow "$workflow" '
      {id:(if $workflow=="go.yml" then 1 else 2 end),run_attempt:2,
        event:"pull_request",head_sha:$head,head_branch:$branch,head_repository:{id:902526165},
        pull_requests:[{number:123,base:{ref:"main"}}], actor:{id:362053602},status:"completed",conclusion:"success"}
      | if $scenario=="stale-ci" then .head_sha="old"
        elif $scenario=="manual-ci" then .event="workflow_dispatch"
        elif $scenario=="human-ci" then .actor.id=42
        elif $scenario=="newer-pending" then [.,(.|.id=3|.status="in_progress"|.conclusion=null)]
        else [.] end
      | if type=="object" then [.] else . end
      | [{workflow_runs:.}]
    ' ;;
  repos/apstndb/spanemuboost/actions/runs/*/jobs)
    case "$2" in
      */runs/1/*) names='["build-and-test","omni-smoke","podman-smoke"]' ;;
      */runs/2/*) names='["lint"]' ;;
      *) exit 7 ;;
    esac
    jq -n --argjson names "$names" --arg head "$HEAD_SHA" --arg scenario "$SCENARIO" '
      [{jobs:[$names[] | {name:.,status:"completed",conclusion:"success",head_sha:$head,run_attempt:2}]}]
      | if $scenario=="failed-job" then .[0].jobs[0].conclusion="failure"
        elif $scenario=="missing-job" then del(.[0].jobs[0])
        elif $scenario=="wrong-attempt" then .[0].jobs[].run_attempt=1
        elif $scenario=="partial-rerun" and ($names|length)>1
          then .[0].jobs[0].run_attempt=1 | .[0].jobs += [
            (.[0].jobs[1] | .run_attempt=1 | .conclusion="failure")]
        elif $scenario=="later-job-failed"
          then .[0].jobs += [(.[0].jobs[0] | .run_attempt=3 | .conclusion="failure")]
        elif $scenario=="wrong-job-head" then .[0].jobs[0].head_sha="old"
        else . end
    ' ;;
  installation/repositories)
    [[ "$GH_TOKEN" == fixture-app ]]
    if [[ "$SCENARIO" == broad-token ]]; then
      echo '[{"repositories":[{"id":902526165},{"id":5}]}]'
    else
      echo '[{"repositories":[{"id":902526165}]}]'
    fi ;;
  graphql)
    [[ "$GH_TOKEN" == fixture-app ]]
    [[ "$*" == *enablePullRequestAutoMerge* && "$*" == *mergeMethod:SQUASH*
       && "$*" == *"head=$HEAD_SHA"* && "$*" == *id=fixture-node* ]]
    printf 'reserve\n' >>"$MERGE_FIXTURE/writes"
    if [[ "$SCENARIO" == reservation-rejected ]]; then exit 7; fi
    jq -n --arg head "$HEAD_SHA" '
      {data:{enablePullRequestAutoMerge:{pullRequest:{headRefOid:$head,autoMergeRequest:{mergeMethod:"SQUASH"}}}}}
    ' ;;
  repos/apstndb/spanemuboost/statuses/*)
    [[ "$GH_TOKEN" == fixture-status && "$2" == "repos/apstndb/spanemuboost/statuses/$HEAD_SHA" ]]
    [[ "$*" == *context=runtime-image-merge-policy* && "$*" == *state=success* ]]
    if [[ "$SCENARIO" != ordinary && "$SCENARIO" != armed ]]; then
      [[ $(cat "$MERGE_FIXTURE/writes") == reserve ]]
    fi
    printf 'status\n' >>"$MERGE_FIXTURE/writes" ;;
  *) exit 7 ;;
esac
STUB
chmod +x "$fixture/gh"

plan=$(bash "$root/scripts/merge-runtime-image-pr.sh")
jq -e --arg head "$HEAD_SHA" --arg base "$BASE_SHA" \
  '. == {number:123,node:"fixture-node",head:$head,base:$base}' <<<"$plan" >/dev/null
test ! -e "$fixture/writes"
for scenario in no-pr draft other-bot fork unmarked rename extra-path stale-base stale-ci manual-ci \
  human-ci newer-pending failed-job missing-job wrong-attempt wrong-job-head later-job-failed; do
  result=$(SCENARIO=$scenario bash "$root/scripts/merge-runtime-image-pr.sh" "$plan")
  [[ -z "$result" && ! -e "$fixture/writes" ]]
done
stale_plan=$(jq -c '.head="old"' <<<"$plan")
[[ -z "$(bash "$root/scripts/merge-runtime-image-pr.sh" "$stale_plan")" ]]
[[ -z "$(EVENT_HEAD=old bash "$root/scripts/merge-runtime-image-pr.sh" "$plan")" ]]
for scenario in broad-token no-required-gate; do
  if SCENARIO=$scenario bash "$root/scripts/merge-runtime-image-pr.sh" "$plan"; then
    echo "Accepted ineligible scenario $scenario" >&2
    exit 1
  fi
  test ! -e "$fixture/writes"
done
if SCENARIO=reservation-rejected bash "$root/scripts/merge-runtime-image-pr.sh" "$plan"; then
  echo 'Published the required status after a failed reservation' >&2
  exit 1
fi
[[ $(cat "$fixture/writes") == reserve ]]
rm -- "$fixture/writes"
[[ "$(SCENARIO=partial-rerun bash "$root/scripts/merge-runtime-image-pr.sh")" == "$plan" ]]
SCENARIO=pass bash "$root/scripts/merge-runtime-image-pr.sh" "$plan"
[[ $(cat "$fixture/writes") == $'reserve\nstatus' ]]
rm -- "$fixture/writes"
SCENARIO=armed bash "$root/scripts/merge-runtime-image-pr.sh" "$plan"
[[ $(cat "$fixture/writes") == status ]]
rm -- "$fixture/writes"
GITHUB_EVENT_NAME=pull_request_target EVENT_PR=123 SCENARIO=copied-tip \
  bash "$root/scripts/merge-runtime-image-pr.sh"
test ! -e "$fixture/writes"
GITHUB_EVENT_NAME=pull_request_target EVENT_PR=123 SCENARIO=ordinary \
  bash "$root/scripts/merge-runtime-image-pr.sh"
[[ $(cat "$fixture/writes") == status ]]
printf 'Offline native auto-merge gate scenarios passed\n'
