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
// selection is the default: tags containing 2026.r1 through r3 (also spelled
// 2026-r1 through r3) use the flagless start-single-server command. The suffix
// need not be beta, and a tag is used even when followed by a digest. All
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
// Digest-only references and unrecognized tags use GA defaults. Selection uses
// the tag's name without inspecting the resolved image. A named tag may be
// retained before @sha256:... to select startup for a pinned older image.
// [WithContainerCustomizers] can override container commands when needed.
// Image selection affects new containers only; attached endpoints and reopened
// clients are unchanged.
package spanemuboost
