package spanemuboost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dcontainer "github.com/moby/moby/api/types/container"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ImageProvenanceSource says how image evidence was obtained.
type ImageProvenanceSource string

const (
	// ImageProvenanceUnverified means the handle has no image evidence.
	// Attached endpoints without producer-reported provenance use this source.
	ImageProvenanceUnverified ImageProvenanceSource = "unverified"
	// ImageProvenanceContainerInspect means this process inspected the owned container.
	ImageProvenanceContainerInspect ImageProvenanceSource = "container_inspect"
	// ImageProvenanceEndpointFile means the fields were read from producer-reported
	// endpoint metadata. That is not a new observation of the running endpoint.
	ImageProvenanceEndpointFile ImageProvenanceSource = "endpoint_file"
)

const (
	dockerMediaTypeManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"
	dockerMediaTypeManifest     = "application/vnd.docker.distribution.manifest.v2+json"
)

// ImageProvenance is backend-neutral image evidence for a package-owned runtime.
//
// RequestedImage is the image reference that was requested, either by this
// process or by the producer of an endpoint file. It is not evidence of the
// image that is running. ImageID, IndexDigest, and ManifestDigest are different
// identifiers. An empty string means that identifier was not available. An
// image ID is never copied into either digest field.
//
// ObservedByThisProcess is true only when Source is
// [ImageProvenanceContainerInspect]. Reading an endpoint file does not promote
// its contents to a fresh observation.
//
// Evidence capture:
//
//	prov, err := spanemuboost.RuntimeImageProvenance(ctx, runtime)
//	if err != nil {
//		return err
//	}
//	if !prov.ObservedByThisProcess {
//		// RequestedImage and any digests are unverified or producer-reported.
//	}
//	fmt.Println(prov.Source, prov.ImageID, prov.ManifestDigest, prov.Platform)
type ImageProvenance struct {
	RequestedImage        string
	ImageID               string
	IndexDigest           string
	ManifestDigest        string
	Platform              string
	ObservedAt            time.Time
	Source                ImageProvenanceSource
	ObservedByThisProcess bool
}

// EndpointProvenance is optional producer-reported image metadata stored in an
// endpoint file. Loading it does not mean this process inspected the container.
type EndpointProvenance struct {
	RequestedImage string    `json:"requested_image,omitempty"`
	ImageID        string    `json:"image_id,omitempty"`
	IndexDigest    string    `json:"index_digest,omitempty"`
	ManifestDigest string    `json:"manifest_digest,omitempty"`
	Platform       string    `json:"platform,omitempty"`
	ObservedAt     time.Time `json:"observed_at,omitempty"`
}

// RuntimeImageProvenance returns image evidence for a package-provided runtime.
//
// Owned Emulator and Omni runtimes return the observation captured from their
// container. Attached runtimes return producer-reported endpoint provenance, or
// [ImageProvenanceUnverified] when the endpoint has none. An unverified attached
// runtime is not an error. [RuntimePlatform] is unchanged.
//
// It accepts the same handles as [RuntimePlatform] and starts lazy handles on
// first use.
func RuntimeImageProvenance(ctx context.Context, runtime RuntimeHandle) (ImageProvenance, error) {
	resolved, err := resolveRuntime(ctx, runtime)
	if err != nil {
		return ImageProvenance{}, err
	}
	switch r := resolved.(type) {
	case *Emulator:
		return r.imageProvenance()
	case *omniRuntime:
		return r.imageProvenance()
	case *AttachedRuntime:
		return r.imageProvenance()
	default:
		return ImageProvenance{}, fmt.Errorf("spanemuboost: unsupported runtime type %T", resolved)
	}
}

func endpointProvenanceFromRuntime(runtime Runtime) *EndpointProvenance {
	switch r := runtime.(type) {
	case *Emulator:
		return r.provenance.endpointSnapshot()
	case *omniRuntime:
		return r.provenance.endpointSnapshot()
	default:
		// Attached and other handles do not turn producer-reported or missing
		// metadata into a new container observation.
		return nil
	}
}

func (e *Emulator) imageProvenance() (ImageProvenance, error) {
	if e == nil || e.provenance == nil {
		return ImageProvenance{}, errors.New("spanemuboost: emulator image provenance is unavailable")
	}
	return *e.provenance, nil
}

func (o *omniRuntime) imageProvenance() (ImageProvenance, error) {
	if o == nil || o.provenance == nil {
		return ImageProvenance{}, errors.New("spanemuboost: omni image provenance is unavailable")
	}
	return *o.provenance, nil
}

func (a *AttachedRuntime) imageProvenance() (ImageProvenance, error) {
	if a == nil || a.reportedProvenance == nil {
		return ImageProvenance{Source: ImageProvenanceUnverified}, nil
	}
	return imageProvenanceFromEndpoint(*a.reportedProvenance), nil
}

func (e *Emulator) captureImageProvenance(ctx context.Context) {
	if e == nil || e.container == nil || e.opts == nil {
		return
	}
	e.provenance = observeContainerImage(ctx, e.container, e.opts.emulatorImage)
}

func (o *omniRuntime) captureImageProvenance(ctx context.Context) {
	if o == nil || o.container == nil || o.opts == nil {
		return
	}
	o.provenance = observeContainerImage(ctx, o.container, o.opts.emulatorImage)
}

// imageInspector is the inspect surface used at startup. A canceled or
// deadline-bound caller context must reach Inspect.
type imageInspector interface {
	Inspect(context.Context) (*dcontainer.InspectResponse, error)
}

func observeContainerImage(ctx context.Context, container imageInspector, requestedImage string) *ImageProvenance {
	if container == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Bound the optional inspect, but derive that bound from the caller so
	// startup cancellation still interrupts it. Inspect errors stay non-fatal.
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	info, err := container.Inspect(inspectCtx)
	if err != nil {
		return nil
	}
	prov, ok := imageProvenanceFromInspect(info, requestedImage, time.Now().UTC())
	if !ok {
		return nil
	}
	return &prov
}

func imageProvenanceFromInspect(info *dcontainer.InspectResponse, requestedImage string, observedAt time.Time) (ImageProvenance, bool) {
	if info == nil {
		return ImageProvenance{}, false
	}
	indexDigest, manifestDigest := classifyManifestDigest(info.ImageManifestDescriptor)
	platform, err := inspectContainerPlatform(info)
	if err != nil {
		platform = ""
	}
	return ImageProvenance{
		RequestedImage:        strings.TrimSpace(requestedImage),
		ImageID:               strings.TrimSpace(info.Image),
		IndexDigest:           indexDigest,
		ManifestDigest:        manifestDigest,
		Platform:              platform,
		ObservedAt:            observedAt.UTC(),
		Source:                ImageProvenanceContainerInspect,
		ObservedByThisProcess: true,
	}, true
}

func classifyManifestDigest(desc *ocispec.Descriptor) (indexDigest, manifestDigest string) {
	if desc == nil || desc.Digest == "" {
		return "", ""
	}
	digest := desc.Digest.String()
	switch desc.MediaType {
	case ocispec.MediaTypeImageIndex, dockerMediaTypeManifestList:
		return digest, ""
	case ocispec.MediaTypeImageManifest, dockerMediaTypeManifest:
		return "", digest
	default:
		// Unknown media type: do not guess index versus platform manifest.
		return "", ""
	}
}

func (p *ImageProvenance) endpointSnapshot() *EndpointProvenance {
	if p == nil || p.Source != ImageProvenanceContainerInspect {
		return nil
	}
	snapshot := EndpointProvenance{
		RequestedImage: p.RequestedImage,
		ImageID:        p.ImageID,
		IndexDigest:    p.IndexDigest,
		ManifestDigest: p.ManifestDigest,
		Platform:       p.Platform,
		ObservedAt:     p.ObservedAt,
	}
	return &snapshot
}

func imageProvenanceFromEndpoint(reported EndpointProvenance) ImageProvenance {
	return ImageProvenance{
		RequestedImage:        reported.RequestedImage,
		ImageID:               reported.ImageID,
		IndexDigest:           reported.IndexDigest,
		ManifestDigest:        reported.ManifestDigest,
		Platform:              reported.Platform,
		ObservedAt:            reported.ObservedAt,
		Source:                ImageProvenanceEndpointFile,
		ObservedByThisProcess: false,
	}
}

func cloneEndpointProvenance(in *EndpointProvenance) *EndpointProvenance {
	if in == nil {
		return nil
	}
	clone := *in
	return &clone
}
