package spanemuboost

import (
	"cmp"
	"fmt"
	"math/rand/v2"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"google.golang.org/api/option"
)

type emulatorOptions struct {
	emulatorImage                     string
	projectID, instanceID, databaseID string

	randomProjectID, randomInstanceID, randomDatabaseID bool

	disableCreateInstance bool
	disableCreateDatabase bool

	databaseDialect        databasepb.DatabaseDialect
	setupDDLs              []string
	setupDMLs              []spanner.Statement
	clientConfig           spanner.ClientConfig
	containerCustomizers   []testcontainers.ContainerCustomizer
	clientOptionsForClient []option.ClientOption
}

type Option func(*emulatorOptions) error

// WithContainerCustomizers sets any testcontainers.ContainerCustomizer
func WithContainerCustomizers(containerCustomizers ...testcontainers.ContainerCustomizer) Option {
	return func(opts *emulatorOptions) error {
		opts.containerCustomizers = append(opts.containerCustomizers, containerCustomizers...)
		return nil
	}
}

// EnableFaultInjection enables fault injection of Cloud Spanner Emulator.
func EnableFaultInjection() Option {
	return func(opts *emulatorOptions) error {
		opts.containerCustomizers = append(opts.containerCustomizers, testcontainers.WithConfigModifier(func(config *container.Config) {
			config.Cmd = append(config.Cmd, "--enable_fault_injection")
		}))
		return nil
	}
}

// WithClientOptionsForClient configures ClientOption for Clients.Client.
func WithClientOptionsForClient(option ...option.ClientOption) Option {
	return func(opts *emulatorOptions) error {
		opts.clientOptionsForClient = append(opts.clientOptionsForClient, option...)
		return nil
	}
}

// WithProjectID configures the project ID.
// Empty string resets to default.
func WithProjectID(projectID string) Option {
	return func(opts *emulatorOptions) error {
		opts.projectID = projectID
		return nil
	}
}

// WithRandomProjectID enables the random project ID. Default is disabled.
func WithRandomProjectID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomProjectID = true
		return nil
	}
}

// WithoutRandomProjectID disables the random project ID. Default is disabled.
func WithoutRandomProjectID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomProjectID = false
		return nil
	}
}

// WithInstanceID configures the instance ID.
// Empty string resets to default.
func WithInstanceID(instanceID string) Option {
	return func(opts *emulatorOptions) error {
		opts.instanceID = instanceID
		return nil
	}
}

// WithRandomInstanceID enables the random instance ID. Default is disabled.
func WithRandomInstanceID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomInstanceID = true
		return nil
	}
}

// WithoutRandomInstanceID disables the random instance ID. Default is disabled.
func WithoutRandomInstanceID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomInstanceID = false
		return nil
	}
}

// WithDatabaseID configures the database ID.
// Empty string resets to default.
func WithDatabaseID(databaseID string) Option {
	return func(opts *emulatorOptions) error {
		opts.databaseID = databaseID
		return nil
	}
}

// WithRandomDatabaseID enables the random database ID. Default is disabled.
func WithRandomDatabaseID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomDatabaseID = true
		return nil
	}
}

// WithoutRandomDatabaseID disables the random database ID. Default is disabled.
func WithoutRandomDatabaseID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomDatabaseID = false
		return nil
	}
}

// WithDatabaseDialect configures the database dialect.
func WithDatabaseDialect(dialect databasepb.DatabaseDialect) Option {
	return func(opts *emulatorOptions) error {
		opts.databaseDialect = dialect
		return nil
	}
}

// WithEmulatorImage configures the Spanner Emulator container image.
// Empty string will be ignored.
func WithEmulatorImage(image string) Option {
	return func(opts *emulatorOptions) error {
		opts.emulatorImage = image
		return nil
	}
}

// WithClientConfig sets spanner.ClienConfig for NewClients and NewEmulatorWithClients.
func WithClientConfig(config spanner.ClientConfig) Option {
	return func(opts *emulatorOptions) error {
		opts.clientConfig = config
		return nil
	}
}

// WithSetupDDLs sets DDLs to be executed.
func WithSetupDDLs(ddls []string) Option {
	return func(opts *emulatorOptions) error {
		opts.setupDDLs = ddls
		return nil
	}
}

// WithSetupRawDMLs sets string DMLs to be executed.
func WithSetupRawDMLs(rawDMLs []string) Option {
	return func(opts *emulatorOptions) error {
		dmlStmts := make([]spanner.Statement, 0, len(rawDMLs))
		for _, rawDML := range rawDMLs {
			dmlStmts = append(dmlStmts, spanner.NewStatement(rawDML))
		}

		opts.setupDMLs = dmlStmts
		return nil
	}
}

// WithSetupDMLs sets DMLs in spanner.Statement to be executed.
func WithSetupDMLs(dmls []spanner.Statement) Option {
	return func(opts *emulatorOptions) error {
		opts.setupDMLs = dmls
		return nil
	}
}

// DisableAutoConfig disables auto config.(default enable)
func DisableAutoConfig() Option {
	return func(opts *emulatorOptions) error {
		opts.disableCreateInstance = true
		opts.disableCreateDatabase = true
		return nil
	}
}

// EnableAutoConfig enables auto config.(default enable)
func EnableAutoConfig() Option {
	return func(opts *emulatorOptions) error {
		opts.disableCreateInstance = false
		opts.disableCreateDatabase = false
		return nil
	}
}

func EnableInstanceAutoConfigOnly() Option {
	return func(opts *emulatorOptions) error {
		opts.disableCreateInstance = false
		opts.disableCreateDatabase = true
		return nil
	}
}

func EnableDatabaseAutoConfigOnly() Option {
	return func(opts *emulatorOptions) error {
		opts.disableCreateInstance = true
		opts.disableCreateDatabase = false
		return nil
	}
}

func (o *emulatorOptions) DatabasePath() string {
	return databasePath(o.projectID, o.instanceID, o.databaseID)
}

func (o *emulatorOptions) InstancePath() string {
	return instancePath(o.projectID, o.instanceID)
}

func (o *emulatorOptions) ProjectPath() string {
	return projectPath(o.projectID)
}

func applyOptions(options ...Option) (*emulatorOptions, error) {
	opts := &emulatorOptions{}

	for _, opt := range options {
		if err := opt(opts); err != nil {
			return nil, err
		}
	}

	if opts.randomProjectID && opts.projectID != "" {
		return nil, fmt.Errorf("WithRandomProjectID() and WithProjectID() are mutually exclusive")
	}

	if opts.randomInstanceID && opts.instanceID != "" {
		return nil, fmt.Errorf("WithRandomInstanceID() and WithInstanceID() are mutually exclusive")
	}

	if opts.randomDatabaseID && opts.databaseID != "" {
		return nil, fmt.Errorf("WithRandomDatabaseID() and WithDatabaseID() are mutually exclusive")
	}

	if opts.randomProjectID {
		opts.projectID = generateRandomID()
	}

	if opts.randomInstanceID {
		opts.instanceID = generateRandomID()
	}

	if opts.randomDatabaseID {
		opts.databaseID = generateRandomID()
	}

	opts.emulatorImage = cmp.Or(opts.emulatorImage, DefaultEmulatorImage)
	opts.projectID = cmp.Or(opts.projectID, DefaultProjectID)
	opts.instanceID = cmp.Or(opts.instanceID, DefaultInstanceID)
	opts.databaseID = cmp.Or(opts.databaseID, DefaultDatabaseID)

	return opts, nil
}

const (
	databaseIDFirstChars = "abcdefghjiklkmnopqrstuvwxyz"
	databaseIDChars      = databaseIDFirstChars + "0123456789"
	idRange              = 30
)

// generateRandomID generates a random database ID.
// Generated ID will be this format: [a-z][a-z0-9]{29}
func generateRandomID() string {
	b := make([]byte, idRange)
	b[0] = databaseIDFirstChars[rand.N(len(databaseIDFirstChars))]
	for i := range idRange - 1 {
		b[i+1] = databaseIDChars[rand.N(len(databaseIDChars))]
	}
	return string(b)
}
