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
// [BackendOmni] defaults to the Developer image 2026.r4-lts and starts it with
// --listen-addresses=0.0.0.0 for container port forwarding. Omni support in this
// package remains experimental regardless of the image's LTS designation.
//
// To retain the old flagless startup command for 2026.r2.1-beta or 2026.r3-beta,
// pair [WithContainerImage] with an explicitly empty [WithOmniStartArgs]:
//
//	env, err := RunWithClients(ctx, BackendOmni,
//		WithContainerImage("us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r3-beta"),
//		WithOmniStartArgs(),
//	)
//
// WithOmniStartArgs replaces all default arguments after start-single-server.
// Custom arguments for r4 must retain the listen address. No version inference
// is made from tags or digests. These options apply to new containers only;
// attached endpoints and clients opened on an existing runtime are unchanged.
// The CLI equivalent is serve omni --image IMAGE --omni-start-mode legacy.
package spanemuboost
