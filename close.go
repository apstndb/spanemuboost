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

func (s *closeState) close(fn func() error) error {
	s.once.Do(func() {
		s.err = fn()
	})
	return s.err
}

func newCloseContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), closeTimeout)
}
