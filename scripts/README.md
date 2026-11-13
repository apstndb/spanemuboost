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
CI checks that the generated file matches the data. Discovery reads the data
file directly; the generated Go API names and values remain unchanged.

## Scheduled update pull requests

The workflow runs daily at 00:17 UTC (09:17 JST) and supports manual dispatch on
the default branch. It creates no Issues. Drift creates or updates one managed
PR on `automation/runtime-images`; matching constants do nothing, and errors
fail the workflow. Full detection JSON is retained for 14 days.

Identical candidate sets create no additional commit or PR. Closing an unmerged
managed PR dismisses that exact candidate set. Different candidates update an
open PR or create a new one after closure. Branch history is additive: current
main is merged before regeneration, never force-pushed. Only runtime-images.json
and images_generated.go may differ on the managed branch. Every existing
branch must also have a managed tip commit marker. Manual tips or unrelated
changes stop automation, including unchanged-candidate CI reconciliation. A pushed orphan branch
can be recovered only when its commit carries the managed marker.

The updater never approves, merges or releases a PR. It requires contents,
pull-request and Actions write permissions and the repository setting allowing
GitHub Actions to create pull requests. Standard GitHub settings couple this
with permission to approve PRs; this workflow does not use approval capability.
The repository default token policy can stay read-only for other workflows.

Events made by GITHUB_TOKEN do not reliably start ordinary CI automatically.
The updater explicitly dispatches the Go and lint workflows at the bot branch.
It checks existing dispatches by head SHA to avoid repeating CI on unchanged
heads and to recover a partially failed dispatch. Depending on GitHub policy,
PR-event runs may separately await approval; explicit dispatch is independent.
Offline tests use isolated git remotes and fake GitHub clients, never real PRs.

Optional GH_TOKEN (then GITHUB_TOKEN) is used only for GitHub release metadata,
not OCI registries or Omni docs. Bash/curl/Go/git are provided by the runner;
crane, jq and gh remain pinned through mise. Before merging an update, inspect
all runtime CI and review the chosen versions. Omni remains experimental.
