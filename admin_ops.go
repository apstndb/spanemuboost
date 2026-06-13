package spanemuboost

import (
	"strings"
	"sync"
)

var (
	instanceAdminMu    sync.Mutex
	instanceAdminLocks = make(map[string]*sync.Mutex)
)

func withInstanceAdminLock(instancePath string, fn func()) {
	if instancePath == "" {
		fn()
		return
	}
	instanceAdminMu.Lock()
	mu, ok := instanceAdminLocks[instancePath]
	if !ok {
		mu = &sync.Mutex{}
		instanceAdminLocks[instancePath] = mu
	}
	instanceAdminMu.Unlock()

	mu.Lock()
	defer mu.Unlock()
	fn()
}

func instancePathFromDatabasePath(dbPath string) string {
	const marker = "/databases/"
	if i := strings.Index(dbPath, marker); i >= 0 {
		return dbPath[:i]
	}
	return dbPath
}
