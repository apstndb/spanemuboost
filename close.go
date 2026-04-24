package spanemuboost

import (
	"context"
	"sync"
	"time"
)

const closeTimeout = 30 * time.Second

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
