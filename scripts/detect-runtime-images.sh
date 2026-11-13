#!/usr/bin/env bash
# mise run images:detect / images:check. Bash 3.2+, crane, jq and curl.
# Exit 0: discovery succeeded (or --check matched), 1: drift with --check,
# 2: invalid invocation, missing dependency or discovery failure.
set -Eeuo pipefail
trap 'printf "discovery failed (line %s)\n" "$LINENO" >&2; exit 2' ERR
check=false
case "${1:-}" in
  --check) check=true ;;
  --help|-h) printf 'Usage: mise run images:detect | images:check\n'; exit 0 ;;
  '') ;;
  *) printf 'Unknown argument: %s\n' "$1" >&2; exit 2 ;;
esac
if (( $# > 1 )); then printf 'Too many arguments\n' >&2; exit 2; fi
for tool in crane jq curl; do
  if ! command -v "$tool" >/dev/null; then
    printf 'Missing dependency: %s (use the mise task)\n' "$tool" >&2
    exit 2
  fi
done
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/spanemuboost-images.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 2' INT TERM
omni=us-docker.pkg.dev/spanner-omni/images/spanner-omni
emulator=gcr.io/cloud-spanner-emulator/emulator
notes=https://docs.cloud.google.com/spanner-omni/release-notes
release=https://api.github.com/repos/GoogleCloudPlatform/cloud-spanner-emulator/releases/latest

# crane handles registry authentication and tag pagination; no Docker daemon.
crane ls "$omni" > "$scratch/omni.tags"
crane ls "$emulator" > "$scratch/emulator.tags"
jq -Rn '[inputs]' < "$scratch/omni.tags" > "$scratch/omni.json"
jq -Rn '[inputs]' < "$scratch/emulator.tags" > "$scratch/emulator.json"
jq -n -L "$root/scripts" --slurpfile omni "$scratch/omni.json" \
  --slurpfile emulator "$scratch/emulator.json" \
  'include "runtime-images"; select_tags($omni[0]; $emulator[0])' > "$scratch/tags.json"
curl --fail --silent --show-error --location --connect-timeout 10 --max-time 30 \
  -H 'User-Agent: spanemuboost-image-discovery' "$notes" > "$scratch/notes.html"
# Use the Actions token when provided to avoid the anonymous GitHub API limit.
# It is sent only to the fixed GitHub API URL, never to OCI registries or docs.
release_headers=(-H 'User-Agent: spanemuboost-image-discovery' -H 'Accept: application/json')
if [[ -n "${GH_TOKEN:-${GITHUB_TOKEN:-}}" ]]; then
  release_headers+=(-H "Authorization: Bearer ${GH_TOKEN:-${GITHUB_TOKEN:-}}")
fi
curl --fail --silent --show-error --location --connect-timeout 10 --max-time 30 \
  "${release_headers[@]}" "$release" > "$scratch/release.json"

for name in DefaultOmniImage LatestLTSOmniImage DefaultEmulatorImage; do
  repo=$omni
  if [[ "$name" == DefaultEmulatorImage ]]; then repo=$emulator; fi
  tag=$(jq -er --arg name "$name" '.[$name]' "$scratch/tags.json")
  digest=$(crane digest "$repo:$tag")
  # Resolve once, then inspect by digest so a tag move cannot mix observations.
  crane manifest "$repo@$digest" > "$scratch/manifest.json"
  jq -n -L "$root/scripts" --arg repo "$repo" --arg tag "$tag" \
    --arg digest "$digest" --slurpfile manifest "$scratch/manifest.json" \
    'include "runtime-images"; image_evidence($repo; $tag; $digest; $manifest[0])' \
    > "$scratch/$name.json"
done

jq -n -L "$root/scripts" \
  --slurpfile current "$root/runtime-images.json" \
  --rawfile notes "$scratch/notes.html" --arg notesUrl "$notes" \
  --slurpfile release "$scratch/release.json" \
  --slurpfile omni "$scratch/DefaultOmniImage.json" \
  --slurpfile lts "$scratch/LatestLTSOmniImage.json" \
  --slurpfile emulator "$scratch/DefaultEmulatorImage.json" \
  'include "runtime-images";
   report($current[0]; $notes; $notesUrl; $release[0];
          $omni[0]; $lts[0]; $emulator[0])' > "$scratch/result.json"
# Print only a complete report. Network/parsing failures never emit partial JSON.
cat "$scratch/result.json"
if [[ "$check" == true ]] && jq -e '.images | any(.update_available)' "$scratch/result.json" >/dev/null; then
  exit 1
fi
