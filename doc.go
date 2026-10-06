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
// [BackendOmni] defaults to the Developer image 2026.r4-lts. Automatic image
// selection is the default: pre-GA beta tags (2026.r1 through r3, including
// dotted patch versions) use the flagless start-single-server command. All
// other references use --listen-addresses=0.0.0.0 for container port forwarding.
// Later beta releases use these GA defaults too. Omni support remains
// experimental regardless of the image's LTS designation.
//
// Select another image with [WithContainerImage]:
//
//	env, err := RunWithClients(ctx, BackendOmni,
//		WithContainerImage("us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r3-beta"),
//	)
//
// The CLI equivalent is serve omni --image IMAGE. Startup uses the image's
// native entrypoint without running help probes or injecting a shell wrapper.
// Digest references and unrecognized tags use GA defaults; their engine version
// is not inferred. Use [WithOmniStartArgs] to supply exact startup arguments,
// including an empty list for a pre-GA digest or custom alias. The CLI supports
// --omni-start-mode auto|ga|legacy, with auto as the default. Manual settings
// override image selection but do not select a different image. Custom r4 args
// must retain the listen address. These options affect new containers only;
// attached endpoints and reopened clients are unchanged.
package spanemuboost
