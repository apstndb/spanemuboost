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


## Scheduled update notifications

The `Runtime image updates` GitHub Actions workflow runs `mise run images:check`
daily at 00:17 UTC (09:17 JST), and supports manual `workflow_dispatch` runs.
GitHub schedules can be delayed. Only the default branch runs the notification
job; manual runs on feature branches are skipped. The workflow has repository
read and issue write permissions and serializes schedule/manual runs to avoid
concurrent duplicate creation.

Matching constants (exit 0) produce no issue. Version drift (exit 1) creates or
updates one tracking issue containing a table of current and available images,
a workflow-run link and a reminder to validate runtime compatibility. Detection
failures (exit 2 or another unexpected nonzero status) fail the workflow, rather
than creating a misleading update issue. Complete discovery JSON is retained
in the `runtime-image-report` artifact for 14 days on successful detection.

The issue publisher (`scripts/notify-runtime-images.sh`) recognizes its own
hidden marker, excludes pull requests and reads all issue pages. It deduplicates
on the changed constant names and current/available references, ignoring times,
run URLs and announcement metadata. Identical candidates cause no issue write;
new candidates update an open tracking issue. A manually closed issue suppresses
that exact candidate set, while a different set can create a new issue. Existing
unmarked issues are untouched. Issues are not automatically closed when constants
catch up, and this workflow never changes code, starts runtimes or releases a
version.

The Actions token is also supplied only to the fixed GitHub release-metadata
request when available (`GH_TOKEN`, then `GITHUB_TOKEN`), reducing anonymous API
rate-limit failures. It is not sent to the public OCI registries or Omni docs.
Offline tests cover notification creation, update, deduplication, dismissal,
pagination, API errors and malformed reports without writing actual issues.
