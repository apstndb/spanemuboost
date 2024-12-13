package spanemuboost

import (
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

type emulatorOptions struct {
	emulatorImage                     string
	projectID, instanceID, databaseID string

	disableCreateInstance bool
	disableCreateDatabase bool

	databaseDialect databasepb.DatabaseDialect
	setupDDLs       []string
	setupDMLs       []spanner.Statement
	clientConfig    spanner.ClientConfig
}

type Option func(*emulatorOptions) error

// WithProjectID configures the project ID.
// Empty string will be ignored.
func WithProjectID(projectID string) Option {
	return func(opts *emulatorOptions) error {
		if projectID != "" {
			opts.projectID = projectID
		}
		return nil
	}
}

// WithInstanceID configures the instance ID.
// Empty string will be ignored.
func WithInstanceID(instanceID string) Option {
	return func(opts *emulatorOptions) error {
		if instanceID != "" {
			opts.instanceID = instanceID
		}
		return nil
	}
}

// WithDatabaseID configures the database ID.
// Empty string will be ignored.
func WithDatabaseID(databaseID string) Option {
	return func(opts *emulatorOptions) error {
		if databaseID != "" {
			opts.databaseID = databaseID
		}
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
		if image != "" {
			opts.emulatorImage = image
		}
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
// Note: comments are not permitted.
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
	opts := &emulatorOptions{
		emulatorImage:         DefaultEmulatorImage,
		projectID:             DefaultProjectID,
		instanceID:            DefaultInstanceID,
		databaseID:            DefaultDatabaseID,
		disableCreateInstance: false,
		disableCreateDatabase: false,
		databaseDialect:       databasepb.DatabaseDialect_DATABASE_DIALECT_UNSPECIFIED,
	}

	for _, opt := range options {
		if err := opt(opts); err != nil {
			return nil, err
		}
	}

	return opts, nil
}
