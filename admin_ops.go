package spanemuboost

import (
	"context"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"
)

var (
	instanceAdminMu    sync.Mutex
	instanceAdminLocks = make(map[string]*semaphore.Weighted)
)

func withInstanceAdminLock(ctx context.Context, instancePath string, fn func()) error {
	if instancePath == "" {
		fn()
		return nil
	}
	sem := instanceAdminSemaphore(instancePath)
	if err := sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer sem.Release(1)
	fn()
	return nil
}

func instanceAdminSemaphore(instancePath string) *semaphore.Weighted {
	instanceAdminMu.Lock()
	defer instanceAdminMu.Unlock()
	sem, ok := instanceAdminLocks[instancePath]
	if !ok {
		sem = semaphore.NewWeighted(1)
		instanceAdminLocks[instancePath] = sem
	}
	return sem
}

func instancePathFromDatabasePath(dbPath string) string {
	const marker = "/databases/"
	if i := strings.Index(dbPath, marker); i >= 0 {
		return dbPath[:i]
	}
	return dbPath
}
