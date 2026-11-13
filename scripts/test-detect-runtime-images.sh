#!/usr/bin/env bash
# Offline behavioral tests. Only Bash, jq and standard Unix utilities run here.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/spanemuboost-images-test.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
mkdir -p "$scratch/repo/scripts" "$scratch/bin"
cp "$root/scripts/detect-runtime-images.sh" "$root/scripts/runtime-images.jq" "$scratch/repo/scripts/"
export PATH="$scratch/bin:$PATH"
cat > "$scratch/bin/crane" <<'STUB'
#!/bin/sh
set -eu
case "${IMAGE_TEST_CASE:-ok}" in
  registry_failure) exit 7 ;;
esac
case "$1" in
  ls)
    case "$2" in
      us-docker.pkg.dev/*)
        case "${IMAGE_TEST_CASE:-ok}" in
          no_candidates) printf '%s\n' latest 2026.r5-beta ;;
          no_lts) printf '%s\n' 2026.r5.1 ;;
          numeric) printf '%s\n' 2026.r5.9 2026.r5.10 2026.r4-lts.2 2026.r4-lts.20 ;;
          lts_latest) printf '%s\n' 2027.r1-lts 2026.r5.1 ;;
          *) printf '%s\n' latest deprecated-public-image-x 2026.r3-beta.9 2026.r5.1 2026.r4-lts.2 2026.r5.1 ;;
        esac ;;
      gcr.io/*)
        case "${IMAGE_TEST_CASE:-ok}" in
          no_emulator) printf '%s\n' latest 1.5.60-beta ;;
          unannounced) printf '%s\n' 1.5.59 1.5.60 ;;
          *) printf '%s\n' 1.5.9 1.5.59 1.5.60-beta latest ;;
        esac ;;
      *) exit 8 ;;
    esac ;;
  digest)
    case "${IMAGE_TEST_CASE:-ok}" in
      digest_failure) exit 8 ;;
      invalid_digest) printf 'not-a-digest\n' ;;
      *) printf 'sha256:%064d\n' 0 ;;
    esac ;;
  manifest)
    # Tests fail if orchestration reads the moving tag instead of the digest.
    case "$2" in *@sha256:*) ;; *) exit 9 ;; esac
    case "${IMAGE_TEST_CASE:-ok}" in
      invalid_json) printf 'not json\n' ;;
      malformed_descriptors) printf '{"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[null]}\n' ;;
      null_descriptors) printf '{"mediaType":"application/vnd.oci.image.index.v1+json","manifests":null}\n' ;;
      unknown_media) printf '{"mediaType":"unknown"}\n' ;;
      single_manifest) printf '{"mediaType":"application/vnd.oci.image.manifest.v1+json"}\n' ;;
      *) printf '%s\n' '{"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"platform":{"os":"linux","architecture":"arm64","variant":"v8"}},{"platform":{"os":"linux","architecture":"amd64"}},{"platform":{"os":"unknown","architecture":"unknown"}},{"platform":{"os":"linux","architecture":"amd64"}}]}' ;;
    esac ;;
  *) exit 10 ;;
esac
STUB
cat > "$scratch/bin/curl" <<'STUB'
#!/bin/sh
set -eu
if [ "${IMAGE_TEST_CASE:-ok}" = http_failure ]; then exit 22; fi
for url do :; done
if [ "${IMAGE_TEST_AUTH:-0}" = 1 ]; then
  case "$url:$*" in
    */release-notes:*Authorization*) exit 11 ;;
    */releases/latest:*"Authorization: Bearer fixture-token"*) ;;
    */releases/latest:*) exit 12 ;;
  esac
fi
case "$url" in
  */release-notes)
    if [ "${IMAGE_TEST_CASE:-ok}" = announced ]; then printf '2026.r5.1 2026.r4-lts.2\n'
    else printf '2026.r5.10 2026.r4-lts.20\n'; fi ;;
  */releases/latest)
    case "${IMAGE_TEST_CASE:-ok}" in
      invalid_release) printf '{"draft":true,"tag_name":"v1.5.59","html_url":"url"}\n' ;;
      malformed_release) printf '[]\n' ;;
      *) printf '{"tag_name":"v1.5.59","draft":false,"prerelease":false,"html_url":"https://example.com/releases/v1.5.59"}\n' ;;
    esac ;;
  *) exit 9 ;;
esac
STUB
chmod +x "$scratch/bin/crane" "$scratch/bin/curl"
constants() {
  printf 'DefaultOmniImage = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:%s"\nLatestLTSOmniImage = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r4-lts.2"\n' "$1" > "$scratch/repo/omni.go"
  printf 'DefaultEmulatorImage = "gcr.io/cloud-spanner-emulator/emulator:1.5.59"\n' > "$scratch/repo/spanemuboost.go"
}
passed=0
run_case() {
  local scenario=$1 expected=$2 actual
  shift 2
  set +e
  IMAGE_TEST_CASE=$scenario bash "$scratch/repo/scripts/detect-runtime-images.sh" "$@" > "$scratch/out" 2> "$scratch/err"
  actual=$?
  set -e
  if [[ "$actual" != "$expected" ]]; then
    printf 'FAIL %s: exit %s, expected %s\n' "$scenario" "$actual" "$expected" >&2
    cat "$scratch/err" >&2
    exit 1
  fi
  if [[ "$expected" == 2 && -s "$scratch/out" ]]; then
    printf 'FAIL %s: failure emitted partial JSON\n' "$scenario" >&2; exit 1
  fi
  passed=$((passed + 1))
}
assert_json() { jq -e "$1" "$scratch/out" >/dev/null; }
constants 2026.r5.1
run_case ok 0
assert_json '.images | length == 3'
assert_json '.images.DefaultOmniImage | .tag == "2026.r5.1" and .update_available == false and .release_notes.tag_mentioned == false and .platforms == ["linux/amd64","linux/arm64/v8"] and .manifest_digest == null and .index_digest != null and .runtime_validation == "not_run"'
assert_json '.images.DefaultEmulatorImage.github_release.matches_registry_latest == true'
run_case ok 0 --check
GH_TOKEN=fixture-token IMAGE_TEST_AUTH=1 run_case ok 0 --check
GH_TOKEN='' GITHUB_TOKEN=fixture-token IMAGE_TEST_AUTH=1 run_case ok 0 --check
constants 2026.r5
run_case ok 0
assert_json '.images.DefaultOmniImage.update_available == true'
run_case ok 1 --check
constants 2026.r5.1
run_case numeric 1 --check
assert_json '.images.DefaultOmniImage.tag == "2026.r5.10" and .images.LatestLTSOmniImage.tag == "2026.r4-lts.20"'
run_case lts_latest 0
assert_json '.images.DefaultOmniImage.tag == "2027.r1-lts" and .images.LatestLTSOmniImage.tag == "2027.r1-lts"'
run_case unannounced 0
assert_json '.images.DefaultEmulatorImage | .tag == "1.5.60" and .github_release.matches_registry_latest == false'
run_case announced 0
assert_json '.images.DefaultOmniImage.release_notes.tag_mentioned and .images.LatestLTSOmniImage.release_notes.tag_mentioned'
run_case single_manifest 0
assert_json '.images.DefaultOmniImage | .index_digest == null and .manifest_digest != null and .platforms == []'
for scenario in no_candidates no_lts no_emulator registry_failure http_failure digest_failure invalid_digest invalid_json malformed_descriptors null_descriptors unknown_media invalid_release malformed_release; do
  run_case "$scenario" 2
done
printf 'not a literal constant\n' > "$scratch/repo/omni.go"
run_case ok 2
constants 2026.r5.1
run_case ok 2 --unknown
run_case ok 2 --check extra
# A restricted PATH guarantees a missing jq without ever reaching real clients.
mkdir "$scratch/missing"
cp "$scratch/bin/crane" "$scratch/bin/curl" "$scratch/missing/"
set +e
PATH="$scratch/missing" /bin/bash "$scratch/repo/scripts/detect-runtime-images.sh" > "$scratch/out" 2> "$scratch/err"
actual=$?
set -e
[[ "$actual" == 2 && ! -s "$scratch/out" ]]
passed=$((passed + 1))
printf '%s offline discovery scenarios passed\n' "$passed"
