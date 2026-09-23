package spanemuboost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dcontainer "github.com/moby/moby/api/types/container"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type fakeImageInspector struct {
	blockUntilDone bool
	info           *dcontainer.InspectResponse
	err            error
}

func (f fakeImageInspector) Inspect(ctx context.Context) (*dcontainer.InspectResponse, error) {
	if f.blockUntilDone {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.info, nil
}

func TestObserveContainerImageHonorsCallerContext(t *testing.T) {
	t.Run("canceled parent", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		got := observeContainerImage(ctx, fakeImageInspector{blockUntilDone: true}, "example:tag")
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Fatalf("canceled inspect took %s, want well under the 10s cap", elapsed)
		}
		if got != nil {
			t.Fatalf("provenance = %#v, want nil", got)
		}
	})
	t.Run("short deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		got := observeContainerImage(ctx, fakeImageInspector{blockUntilDone: true}, "example:tag")
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("short-deadline inspect took %s, want about 50ms", elapsed)
		}
		if got != nil {
			t.Fatalf("provenance = %#v, want nil", got)
		}
	})
	t.Run("inspect failure", func(t *testing.T) {
		got := observeContainerImage(context.Background(), fakeImageInspector{err: errors.New("inspect failed")}, "example:tag")
		if got != nil {
			t.Fatalf("provenance = %#v, want nil after inspect failure", got)
		}
	})
}

func TestImageProvenanceFromInspectDistinguishesDigestKinds(t *testing.T) {
	observedAt := time.Date(2026, 9, 23, 1, 2, 3, 0, time.UTC)
	manifest := &dcontainer.InspectResponse{
		Image:    "sha256:image",
		Platform: "linux",
		ImageManifestDescriptor: &ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageManifest,
			Digest:    "sha256:manifest",
			Platform:  &ocispec.Platform{OS: "linux", Architecture: "arm64"},
		},
	}
	got, ok := imageProvenanceFromInspect(manifest, "example:tag", observedAt)
	if !ok {
		t.Fatal("imageProvenanceFromInspect() ok = false")
	}
	if got.RequestedImage != "example:tag" || got.ImageID != "sha256:image" {
		t.Fatalf("identity = %#v", got)
	}
	if got.ManifestDigest != "sha256:manifest" || got.IndexDigest != "" {
		t.Fatalf("digests = index %q manifest %q", got.IndexDigest, got.ManifestDigest)
	}
	if got.ImageID == got.ManifestDigest {
		t.Fatal("image ID was copied into the manifest digest")
	}
	if got.Platform != "linux/arm64" {
		t.Fatalf("platform = %q", got.Platform)
	}
	if got.Source != ImageProvenanceContainerInspect || !got.ObservedByThisProcess {
		t.Fatalf("source = %q observed = %t", got.Source, got.ObservedByThisProcess)
	}

	index := &dcontainer.InspectResponse{
		Image: "sha256:image",
		ImageManifestDescriptor: &ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageIndex,
			Digest:    "sha256:index",
		},
	}
	got, ok = imageProvenanceFromInspect(index, "example:tag", observedAt)
	if !ok {
		t.Fatal("index inspect ok = false")
	}
	if got.IndexDigest != "sha256:index" || got.ManifestDigest != "" {
		t.Fatalf("index digests = index %q manifest %q", got.IndexDigest, got.ManifestDigest)
	}

	unknown := &dcontainer.InspectResponse{
		Image: "sha256:image",
		ImageManifestDescriptor: &ocispec.Descriptor{
			MediaType: "application/vnd.example.unknown",
			Digest:    "sha256:unknown",
		},
	}
	got, ok = imageProvenanceFromInspect(unknown, "example:tag", observedAt)
	if !ok {
		t.Fatal("unknown media type ok = false")
	}
	if got.IndexDigest != "" || got.ManifestDigest != "" || got.ImageID != "sha256:image" {
		t.Fatalf("unknown media type was classified: %#v", got)
	}

	if _, ok := imageProvenanceFromInspect(nil, "example:tag", observedAt); ok {
		t.Fatal("nil inspect ok = true")
	}
}

func TestEndpointProvenanceRoundTripAndOldFile(t *testing.T) {
	observedAt := time.Date(2026, 9, 23, 4, 5, 6, 0, time.UTC)
	endpoint := Endpoint{
		Backend:    BackendEmulator,
		URI:        "127.0.0.1:9010",
		ProjectID:  DefaultProjectID,
		InstanceID: DefaultInstanceID,
		Provenance: &EndpointProvenance{
			RequestedImage: "example:tag",
			ImageID:        "sha256:image",
			ManifestDigest: "sha256:manifest",
			Platform:       "linux/arm64",
			ObservedAt:     observedAt,
		},
	}
	path := filepath.Join(t.TempDir(), "endpoint.json")
	if err := SaveEndpoint(path, endpoint); err != nil {
		t.Fatalf("SaveEndpoint() error = %v", err)
	}
	loaded, err := ReadEndpointFile(path)
	if err != nil {
		t.Fatalf("ReadEndpointFile() error = %v", err)
	}
	if loaded.URI != endpoint.URI || loaded.Provenance == nil {
		t.Fatalf("loaded = %#v", loaded)
	}
	if !loaded.Provenance.ObservedAt.Equal(observedAt) || loaded.Provenance.RequestedImage != "example:tag" {
		t.Fatalf("provenance = %#v", loaded.Provenance)
	}

	attached, err := NewAttachedRuntime(loaded)
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	prov, err := RuntimeImageProvenance(t.Context(), attached)
	if err != nil {
		t.Fatalf("RuntimeImageProvenance() error = %v", err)
	}
	if prov.Source != ImageProvenanceEndpointFile || prov.ObservedByThisProcess {
		t.Fatalf("loaded provenance source = %q observed = %t", prov.Source, prov.ObservedByThisProcess)
	}
	if prov.RequestedImage != "example:tag" || prov.ImageID != "sha256:image" || prov.ManifestDigest != "sha256:manifest" {
		t.Fatalf("producer fields = %#v", prov)
	}

	oldPath := filepath.Join(t.TempDir(), "old.json")
	oldJSON := `{
		"backend": "emulator",
		"uri": "127.0.0.1:9010",
		"project_id": "emulator-project",
		"instance_id": "emulator-instance"
	}`
	if err := os.WriteFile(oldPath, []byte(oldJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := ReadEndpointFile(oldPath)
	if err != nil {
		t.Fatalf("old ReadEndpointFile() error = %v", err)
	}
	if old.Provenance != nil {
		t.Fatalf("old provenance = %#v, want nil", old.Provenance)
	}
	oldAttached, err := NewAttachedRuntime(old)
	if err != nil {
		t.Fatalf("old NewAttachedRuntime() error = %v", err)
	}
	unverified, err := RuntimeImageProvenance(t.Context(), oldAttached)
	if err != nil {
		t.Fatalf("unverified RuntimeImageProvenance() error = %v", err)
	}
	if unverified.Source != ImageProvenanceUnverified || unverified.ObservedByThisProcess {
		t.Fatalf("unverified = %#v", unverified)
	}
	if unverified.RequestedImage != "" || unverified.ImageID != "" || unverified.ManifestDigest != "" || unverified.IndexDigest != "" {
		t.Fatalf("unverified fields were populated: %#v", unverified)
	}
}

func assertObservedRuntimeProvenance(t *testing.T, runtime Runtime, requested string) {
	t.Helper()
	prov, err := RuntimeImageProvenance(t.Context(), runtime)
	if err != nil {
		t.Fatalf("RuntimeImageProvenance() error = %v", err)
	}
	if prov.Source != ImageProvenanceContainerInspect || !prov.ObservedByThisProcess {
		t.Fatalf("source = %q observed = %t", prov.Source, prov.ObservedByThisProcess)
	}
	if prov.RequestedImage != requested {
		t.Fatalf("RequestedImage = %q, want %q", prov.RequestedImage, requested)
	}
	if prov.ImageID == "" && prov.ManifestDigest == "" && prov.IndexDigest == "" {
		t.Fatal("observed image identity is empty")
	}
	if prov.ImageID != "" && (prov.ImageID == prov.ManifestDigest || prov.ImageID == prov.IndexDigest) {
		t.Fatal("image ID was conflated with a digest")
	}
	if strings.TrimSpace(prov.Platform) == "" {
		t.Fatal("platform is empty")
	}
	if prov.ObservedAt.IsZero() {
		t.Fatal("ObservedAt is zero")
	}

	endpoint, err := EndpointFromRuntime(runtime)
	if err != nil {
		t.Fatalf("EndpointFromRuntime() error = %v", err)
	}
	if endpoint.Provenance == nil || endpoint.Provenance.ImageID != prov.ImageID {
		t.Fatalf("endpoint provenance = %#v", endpoint.Provenance)
	}
	attached, err := NewAttachedRuntime(endpoint)
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	reported, err := RuntimeImageProvenance(t.Context(), attached)
	if err != nil {
		t.Fatalf("attached RuntimeImageProvenance() error = %v", err)
	}
	if reported.Source != ImageProvenanceEndpointFile || reported.ObservedByThisProcess {
		t.Fatalf("attached source = %q observed = %t", reported.Source, reported.ObservedByThisProcess)
	}
	if reported.ImageID != prov.ImageID || reported.RequestedImage != prov.RequestedImage {
		t.Fatalf("attached provenance = %#v, live = %#v", reported, prov)
	}
}
