package spanemuboost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

const closeTimeout = 30 * time.Second

const (
	dropDatabaseTimeout     = 2 * time.Minute
	dropDatabaseMaxAttempts = 3
)

func dropDatabaseWithRetry(ctx context.Context, dbCli *database.DatabaseAdminClient, dbPath string) error {
	if dbCli == nil {
		return fmt.Errorf("drop database %s: database admin client is nil", dbPath)
	}
	instancePath := instancePathFromDatabasePath(dbPath)
	var lastErr error
	for attempt := 1; attempt <= dropDatabaseMaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, dropDatabaseTimeout)
		withInstanceAdminLock(instancePath, func() {
			lastErr = dbCli.DropDatabase(attemptCtx, &databasepb.DropDatabaseRequest{Database: dbPath})
		})
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt == dropDatabaseMaxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return errors.Join(fmt.Errorf("drop database %s: %w", dbPath, lastErr), ctx.Err())
		case <-time.After(time.Duration(attempt) * 5 * time.Second):
		}
	}
	return fmt.Errorf("drop database %s after %d attempts: %w", dbPath, dropDatabaseMaxAttempts, lastErr)
}

type closeState struct {
	once sync.Once
	err  error
}

var closeStateInitMu sync.Mutex

func (s *closeState) close(fn func() error) error {
	s.once.Do(func() {
		s.err = fn()
	})
	return s.err
}

// Exported value types keep *closeState rather than embedding sync.Once
// directly so they preserve their previous comparability semantics and do not
// expose a copylock field as part of the public struct layout.
func ensureCloseState(slot **closeState) *closeState {
	closeStateInitMu.Lock()
	defer closeStateInitMu.Unlock()
	if *slot == nil {
		*slot = &closeState{}
	}
	return *slot
}

func newCloseContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), closeTimeout)
}
