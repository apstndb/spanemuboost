# Runtime environments for Spanner Omni tests

This note records runtime settings that are useful for running or rechecking
`spanemuboost` Spanner Omni tests on local runtimes and GitHub Actions.

Last checked: 2026-05-19.

## Status

| Runtime | Machine image | Result |
|---|---|---|
| Colima with Docker | Ubuntu 24.04 VM | Quickstart passed with 4 GiB; `make omni-smoke` passed with 4 GiB; full Omni suite passed with 8 GiB |
| Podman machine | `quay.io/podman/machine-os:5.7` | Quickstart passed with 4 GiB; rootless tests passed with Ryuk disabled; rootful tests passed with Ryuk enabled |
| Podman machine | `quay.io/podman/machine-os:5.8` | Podman VM/API started with 4 GiB rootful, but Spanner Omni `2026.r1-beta` did not become ready; `spanner_server` repeatedly crashed |

Spanner Omni is memory-heavy. Plan for roughly 4 GiB per concurrently running
Omni container. For repository smoke checks, prefer `make omni-smoke`, which
runs with `-p=1 -parallel=1`.

## Colima with Docker

Validated setup:

```sh
colima start --runtime docker --arch aarch64 --vm-type vz \
  --cpus 4 --memory 4 --disk 50 --network-address --save-config
```

Run the smoke tests with:

```sh
env DOCKER_HOST=unix://${HOME}/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  make omni-smoke
```

For a full opt-in Omni run, use a larger Colima profile if possible:

```sh
env DOCKER_HOST=unix://${HOME}/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  SPANEMUBOOST_ENABLE_OMNI_TESTS=1 \
  go test -v -race -count=1 -p=1 -parallel=1 -timeout=20m ./...
```

Notes:

- `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock` keeps Ryuk
  cleanup enabled inside the Colima VM.
- In this local environment, relying only on the active Docker context was not
  enough for Testcontainers-Go; explicit `DOCKER_HOST` was required.
- Colima also supports `containerd` and `incus`, but those paths are not
  validated for this repository's Testcontainers-Go setup.

## Podman machine with machine-os 5.7

Validated rootless setup:

```sh
podman machine init --image docker://quay.io/podman/machine-os:5.7 \
  --cpus 4 --memory 4096 --disk-size 100
podman machine start
```

Rootless test command:

```sh
env DOCKER_HOST=unix://<PodmanSocket.Path from podman machine inspect> \
  TESTCONTAINERS_RYUK_DISABLED=true \
  SPANEMUBOOST_ENABLE_OMNI_TESTS=1 \
  go test -v -race -count=1 -p=1 -parallel=1 -timeout=20m ./...
```

Validated rootful setup:

```sh
podman machine set --rootful
podman machine stop
podman machine start
```

Rootful test command with Ryuk enabled:

```sh
env DOCKER_HOST=unix://<PodmanSocket.Path from podman machine inspect> \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/run/podman/podman.sock \
  TESTCONTAINERS_RYUK_CONTAINER_PRIVILEGED=true \
  SPANEMUBOOST_TESTCONTAINERS_PROVIDER=podman \
  make omni-smoke
```

Why these variables are needed:

- `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/run/podman/podman.sock` lets Ryuk
  mount the Podman socket inside the Linux VM instead of the macOS forwarded
  socket path.
- `TESTCONTAINERS_RYUK_CONTAINER_PRIVILEGED=true` matches Testcontainers-Go's
  Podman guidance for Ryuk.
- `SPANEMUBOOST_TESTCONTAINERS_PROVIDER=podman` forces Testcontainers-Go to use
  Podman's provider behavior and default `podman` network. The macOS forwarded
  socket path can be named `podman-machine-default-api.sock`, which is not enough
  for Testcontainers-Go's automatic Podman detection.

## Podman machine with machine-os 5.8

Rechecked setup:

```sh
podman machine init --image docker://quay.io/podman/machine-os:5.8 \
  --memory 4096 --rootful
podman machine start
```

Observed VM/API state:

```text
Podman client: 5.8.2
Podman server: 5.8.2
Kernel: 6.19.7-200.fc43.aarch64
Rootful: true
Memory: 4096 MiB
Remote socket inside VM: unix:///run/podman/podman.sock
```

The VM and Podman API started successfully, but Spanner Omni did not become
ready. The quickstart container was started with:

```sh
podman volume create spanemuboost-omni-58
podman run -d --network host --name spanemuboost-omni-58 \
  -v spanemuboost-omni-58:/spanner \
  us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r1-beta \
  start-single-server
```

The container stayed up, but the logs repeatedly showed crashes before readiness:

```text
Waiting for Spanner to be ready.
Server server has stopped: failed to run spanner_server: signal: segmentation fault (core dumped)
Server base_services has stopped: failed to run spanner_server: signal: segmentation fault (core dumped)
Server server has stopped: failed to run spanner_server: signal: trace/breakpoint trap (core dumped)
```

The same failure is visible from `spanemuboost`:

```sh
env DOCKER_HOST=unix://<PodmanSocket.Path from podman machine inspect> \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/run/podman/podman.sock \
  TESTCONTAINERS_RYUK_CONTAINER_PRIVILEGED=true \
  SPANEMUBOOST_TESTCONTAINERS_PROVIDER=podman \
  SPANEMUBOOST_ENABLE_OMNI_TESTS=1 \
  go test -v -race -count=1 -timeout=4m -run '^TestRunOmni$' ./...
```

Result:

```text
wait until ready: "Spanner is ready" matched 0 times, expected 1
context deadline exceeded
```

The test logs showed repeated `spanner_server` crashes with `segmentation fault`,
`bus error`, and `aborted` before readiness. Ryuk cleaned up the failed
containers and volumes after the test exited.

## GitHub Actions rootless Podman

GitHub-hosted Ubuntu runners include Podman. For Testcontainers-Go, start a
rootless Podman API service and point `DOCKER_HOST` at that socket. Disable Ryuk
because the hosted runner is ephemeral and the rootless Podman path does not
provide the privileged reaper setup used by local rootful Podman. Run both the
default emulator smoke tests and the Omni smoke tests so CI covers the
Testcontainers-Go Spanner emulator module path as well as the custom Omni
container path.

```sh
export XDG_RUNTIME_DIR="${RUNNER_TEMP}/podman-run"
export DOCKER_HOST="unix://${XDG_RUNTIME_DIR}/podman.sock"
export TESTCONTAINERS_RYUK_DISABLED=true
export SPANEMUBOOST_TESTCONTAINERS_PROVIDER=podman

mkdir -p "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"
podman system service --time=0 "$DOCKER_HOST" &
podman_ready=false
for _ in {1..30}; do
  if podman --url "$DOCKER_HOST" info; then
    podman_ready=true
    break
  fi
  sleep 1
done
if [ "$podman_ready" != true ]; then
  echo "Podman API service did not become ready" >&2
  exit 1
fi

make emulator-smoke
make omni-smoke
```

## Quickstart probe

Use this to recheck a runtime without running Go tests:

```sh
podman volume create spanner
podman run -d --network host --name spanneromni -v spanner:/spanner \
  us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r1-beta \
  start-single-server
podman logs --tail 120 spanneromni
podman exec spanneromni /google/spanner/bin/spanner databases create-sample-db retail --database-name=retail-sample
podman exec spanneromni /google/spanner/bin/spanner databases list
podman rm -f spanneromni
podman volume rm spanner
```

The expected healthy path is:

- logs include `Spanner is ready`
- sample database creation completes
- `databases list` shows the database in `READY` state

For Docker or Colima, replace `podman` with `docker`.

## Diagnostics

Useful commands when a runtime does not behave as expected:

```sh
podman machine list
podman machine inspect
podman info
podman system connection list
podman machine ssh <machine-name> 'uname -a; systemctl status podman.socket --no-pager'
podman ps -a
podman volume ls
```

For Testcontainers-Go failures, also check:

- whether `DOCKER_HOST` points at the intended runtime socket
- whether Ryuk can mount the in-VM socket path
- whether the provider is `podman` when using Podman machine
- whether the Omni container logs reached `Spanner is ready`

## References

- Spanner Omni system requirements: https://docs.cloud.google.com/spanner-omni/system-requirements
- Spanner Omni quickstart: https://docs.cloud.google.com/spanner-omni/quickstart
- Testcontainers-Go with Colima: https://golang.testcontainers.org/system_requirements/using_colima/
- Testcontainers-Go with Podman: https://golang.testcontainers.org/system_requirements/using_podman/
- Testcontainers-Go configuration: https://golang.testcontainers.org/features/configuration/
- Podman machine: https://docs.podman.io/en/latest/markdown/podman-machine.1.html
- GitHub Actions Ubuntu runner image: https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Readme.md
