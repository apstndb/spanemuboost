#!/usr/bin/env bash
# Offline API fixtures; never create or edit actual GitHub issues.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/runtime-image-notify-test.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
mkdir "$scratch/bin"
export NOTIFY_TEST_DIR=$scratch
export PATH="$scratch/bin:$PATH"
export GITHUB_REPOSITORY=apstndb/spanemuboost GITHUB_RUN_ID=42
cat > "$scratch/bin/gh" <<'STUB'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$NOTIFY_TEST_DIR/calls"
if [ "${NOTIFY_API_FAIL:-0}" = 1 ]; then exit 7; fi
case "$*" in
  *--paginate*) cat "$NOTIFY_TEST_DIR/issues.json" ;;
  *)
    method= input=
    while [ "$#" -gt 0 ]; do
      case "$1" in
        --method) method=$2; shift 2 ;;
        --input) input=$2; shift 2 ;;
        *) shift ;;
      esac
    done
    if [ "${NOTIFY_WRITE_FAIL:-0}" = 1 ]; then exit 7; fi
    printf '%s\n' "$method" > "$NOTIFY_TEST_DIR/method"
    cp "$input" "$NOTIFY_TEST_DIR/payload.json"
    printf 'https://github.com/apstndb/spanemuboost/issues/999\n' ;;
esac
STUB
chmod +x "$scratch/bin/gh"
cat > "$scratch/report.json" <<'JSON'
{"images":{"DefaultEmulatorImage":{"current_image":"emulator:1","image":"emulator:2","update_available":true},"DefaultOmniImage":{"current_image":"omni:1","image":"omni:1","update_available":false},"LatestLTSOmniImage":{"current_image":"lts:1","image":"lts:1","update_available":false}}}
JSON
printf '[]\n' > "$scratch/issues.json"
passed=0
run_case() {
  local expected=$1 actual
  rm -f "$scratch/method" "$scratch/calls"
  set +e
  bash "$root/scripts/notify-runtime-images.sh" "$scratch/report.json" > "$scratch/out" 2> "$scratch/err"
  actual=$?
  set -e
  if [[ "$actual" != "$expected" ]]; then cat "$scratch/err" >&2; printf 'Expected %s, got %s\n' "$expected" "$actual" >&2; exit 1; fi
  if [[ "$expected" == 2 && -s "$scratch/out" ]]; then
    printf 'Failure emitted notification output\n' >&2; exit 1
  fi
  passed=$((passed+1))
}
run_case 0
[[ $(cat "$scratch/method") == POST ]]
jq -e '.body|contains("Emulator") and contains("emulator:2") and contains("not a claim of runtime compatibility")' "$scratch/payload.json" >/dev/null
cp "$scratch/payload.json" "$scratch/original.json"
jq '[{number:7,state:"open",body:.body}]' "$scratch/original.json" > "$scratch/issues.json"
run_case 0
[[ ! -e "$scratch/method" ]]
# checked_at/run identity and announcement metadata must not cause daily writes.
GITHUB_RUN_ID=43 run_case 0
[[ ! -e "$scratch/method" ]]
jq '.images.DefaultEmulatorImage.image="emulator:3"' "$scratch/report.json" > "$scratch/changed.json"
mv "$scratch/changed.json" "$scratch/report.json"
run_case 0
[[ $(cat "$scratch/method") == PATCH ]]
grep -q 'repos/apstndb/spanemuboost/issues/7' "$scratch/calls"
# Closing an identical candidate set suppresses recreation.
jq '[{number:7,state:"closed",body:.body}]' "$scratch/payload.json" > "$scratch/issues.json"
run_case 0
[[ ! -e "$scratch/method" ]]
# A dismissed set stays suppressed even when another tracking issue is open.
jq -n --slurpfile closed "$scratch/payload.json" --slurpfile opened "$scratch/original.json" '[{number:7,state:"closed",body:$closed[0].body},{number:8,state:"open",body:$opened[0].body}]' > "$scratch/issues.json"
run_case 0
[[ ! -e "$scratch/method" ]]
# A different candidate set may create a new issue after closure.
jq '[{number:7,state:"closed",body:.body}]' "$scratch/original.json" > "$scratch/issues.json"
run_case 0
[[ $(cat "$scratch/method") == POST ]]
# All pages are merged and PRs cannot be selected as tracking issues.
jq '[{number:1,state:"open",pull_request:{},body:.body}]' "$scratch/payload.json" > "$scratch/issues.json"
jq '[{number:8,state:"open",body:.body}]' "$scratch/payload.json" >> "$scratch/issues.json"
run_case 0
[[ ! -e "$scratch/method" ]]
# Invalid input, API failure and wrong repo fail without a write.
printf '[]\n' > "$scratch/issues.json"
NOTIFY_API_FAIL=1 run_case 2
[[ ! -e "$scratch/method" ]]
NOTIFY_WRITE_FAIL=1 run_case 2
[[ ! -e "$scratch/method" ]]
GITHUB_REPOSITORY=someone/else run_case 2
[[ ! -e "$scratch/calls" ]]
jq '.images.DefaultEmulatorImage.update_available=false' "$scratch/report.json" > "$scratch/inconsistent.json"
mv "$scratch/inconsistent.json" "$scratch/report.json"
run_case 2
[[ ! -e "$scratch/calls" ]]
printf '[]\n' > "$scratch/report.json"
run_case 2
[[ ! -e "$scratch/calls" ]]
printf '{"images":{}}\n' > "$scratch/report.json"
run_case 2
[[ ! -e "$scratch/calls" ]]
# No updates do not even query GitHub.
cat > "$scratch/report.json" <<'JSON'
{"images":{"DefaultEmulatorImage":{"current_image":"e:1","image":"e:1","update_available":false},"DefaultOmniImage":{"current_image":"o:1","image":"o:1","update_available":false},"LatestLTSOmniImage":{"current_image":"l:1","image":"l:1","update_available":false}}}
JSON
run_case 0
[[ ! -e "$scratch/calls" ]]
printf '%s offline notification scenarios passed\n' "$passed"
