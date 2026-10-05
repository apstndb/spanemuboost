// Package spanemuboost starts Cloud Spanner Emulator and experimental Spanner
// Omni runtimes for tests, then bootstraps instances, databases, schema, and
// Spanner clients.
//
// The emulator path is the default and stable path. Use
// [SetupEmulatorWithClients] for simple tests. For many independent cases
// against either backend, share one runtime with [NewLazyRuntime] and create one
// database per case with [WithRandomDatabaseID].
//
// The Omni path uses [BackendOmni] through the backend-neutral APIs such as
// [Setup], [Run], [SetupWithClients], [RunWithClients], [OpenClients],
// [SetupClients], and [NewLazyRuntime]. Omni support is experimental. The
// shared-runtime pattern is especially important for Omni because each started
// Omni runtime owns one Spanner Omni container.
//
// # Omni image selection
//
// [BackendOmni] defaults to the Developer image 2026.r4-lts. Startup checks
// start-single-server --help inside the owned container and adds
// --listen-addresses=0.0.0.0 when supported, so r4 is reachable through the
// published port while older images keep their flagless command. The verified
// 2026.r2.1-beta, 2026.r3-beta, and 2026.r4-lts images share this startup path.
// Omni support remains experimental regardless of the image's LTS designation.
//
// Select another image with [WithContainerImage]:
//
//	env, err := RunWithClients(ctx, BackendOmni,
//		WithContainerImage("us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r3-beta"),
//	)
//
// The CLI equivalent is serve omni --image IMAGE. No version inference is made
// from tags or digests, and detection does not start a second container.
// [WithOmniStartArgs] bypasses detection and uses the image's native entrypoint
// with explicit arguments, for custom layouts or help behavior. Custom r4
// arguments must retain the listen address. These options apply to new
// containers only; attached endpoints and reopened clients are unchanged.
package spanemuboost
