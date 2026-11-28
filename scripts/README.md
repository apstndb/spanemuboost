# Runtime image discovery

Use the mise tasks from the checkout:

```sh
mise run images:detect   # JSON evidence, even if updates are available
mise run images:check    # Exit 1 when public image constants differ
mise run images:test     # Offline selection and orchestration tests
```

`mise.toml` pins crane 0.22.1, jq 1.8.2 and gh 2.104.0 via mise's aqua backend. Mise installs
these tools on first use. The wrappers require Bash 3.2 or newer, curl and
standard Unix utilities (provided by macOS and the Linux CI runners). Python,
a Docker daemon and a registry login are not required. Offline tests use fake
OCI/HTTP clients; tool installation itself can require network access.

The tasks delegate to `scripts/detect-runtime-images.sh`; the same wrapper can
be invoked directly with the dependencies on PATH. Selection and JSON assembly
live in `scripts/runtime-images.jq`. `make image-discovery-test` is retained as
an alias for `mise run images:test`, and hosted CI uses the same mise task.

Crane owns registry authentication, tag pagination, and manifest retrieval for
Omni Developer and Cloud Spanner Emulator. The detector first resolves a tag to
a digest, then reads the manifest by that digest so a tag move cannot combine
different observations. It selects non-prerelease numeric tags, including
patches. Omni versions are ordered by year, release generation and patch, not
upload time or string order. LTS candidates are selected separately; the
overall Latest may also be LTS. Beta and administrative tags are excluded.

JSON output retains the original detector's fields: `checked_at` (UTC), and
`images` keyed by the three public constant names. Each entry includes the
image reference, index or manifest digest, advertised platforms, current value
and drift status. An empty platform list means no platform descriptors were
advertised. Index and manifest digests are not container image IDs. Timestamp
precision may differ between implementations.

The tasks do not start containers, edit constants or publish releases.
Exit codes are preserved through mise:

- 0: successful discovery; with `images:check`, all constants match.
- 1: `images:check` found image-reference drift (JSON is still emitted).
- 2: discovery failure, invalid arguments or missing dependencies. Diagnostics
  go to stderr, and a failed discovery emits no partial JSON to stdout.

This compares tag references, not historical digests of the current tag; it is
not a tag-mutation monitor. `images:detect` returns 0 even when updates exist.

Curl retains the original announcement checks: Omni release-note tag mentions
and the Emulator GitHub latest-release match are reported separately from
registry availability. An unmentioned tag is still an available image, not
evidence of an official announcement. Runtime validation is explicitly
`not_run`, even if the same images were tested elsewhere. Failures fetching
announcement data fail discovery rather than silently dropping those fields.

After selecting new constants, run the repository gates plus `make omni-smoke`
(with `TESTCONTAINERS_RYUK_DISABLED=true` if required). Omni smoke covers the
latest default, latest LTS, and explicitly supported legacy images. Hosted CI
runs it with Docker and rootless Podman. Live detection is an explicit network
operation, separate from the offline tests in the build-and-test job.


## Pinned data and generation

`runtime-images.json` is the single source for the three public image constants.
Run `go generate .` after editing it. Generation uses only the local data and
standard Go library, writes `images_generated.go`, and never queries a registry.
Keep the JSON keys sorted with `jq -S`, as the updater does. CI checks the
canonical JSON and verifies that the generated Go file matches it.

## Scheduled update pull requests

The workflow runs daily at 00:17 UTC (09:17 JST) and supports manual dispatch on
main. Drift creates or updates one App-managed PR on
`automation/runtime-images-octo-sts`. Matching constants leave the repository
unchanged. Full detection JSON is retained for 14 days.

Identical candidate sets create no additional commit or PR. Closing an unmerged
managed PR dismisses that exact candidate set. Different candidates update an
open PR or create a new one after closure. Branch history is additive. The
updater merges main before regenerating the constants from the report.
Only runtime-images.json and images_generated.go may differ on the managed
branch. A PR must have the configured App author and management marker.
An existing branch must have a managed tip commit marker, including when no PR
exists after an interrupted update. Manual tips or unrelated changes stop
automation.

The publish job exchanges its OIDC identity for a repository-scoped Octo STS
token with Contents and Pull requests write permissions. Git pushes and PR API
requests both use that token. The resulting pull_request events start Go and
lint CI.

The auto-merge workflow checks the App author, managed branch and markers.
The head must include current main and modify exactly runtime-images.json
and images_generated.go. The workflow verifies four successful CI jobs at
that head using the latest result of each job in the regular pull_request
runs. It reserves native GitHub squash auto-merge at that exact SHA via
Octo STS and publishes the required image policy status for the same SHA.
GitHub enforces the status plus Go build-and-test, lint, Omni smoke, and
Podman smoke. Ordinary PRs receive the status from the trusted base workflow
and can use manual GitHub auto-merge. Managed commits always require all of
the image-specific checks. Make an update PR a draft to hold it, or manually
dispatch the workflow to re-evaluate its current head. Offline tests exercise
discovery, publication, approval of ordinary PRs, and native reservations.

During detection, optional GH_TOKEN (then GITHUB_TOKEN) authenticates release metadata requests,
not OCI registries or Omni docs. Bash/curl/Go/git are provided by the runner;
crane, jq and gh remain pinned through mise. Omni remains experimental.
