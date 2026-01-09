package tsdns

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type stubRepo struct {
	findCount  atomic.Int32
	closeCount atomic.Int32
}

var errNotImplemented = errors.New("not implemented")

func (r *stubRepo) Find(_ context.Context) ([]*Record, error) {
	r.findCount.Add(1)

	return []*Record{}, nil
}

func (r *stubRepo) FindByDomain(_ context.Context, _ string) (*Record, error) {
	return nil, errNotImplemented
}

func (r *stubRepo) Create(_ context.Context, _ *Record) error {
	return nil
}

func (r *stubRepo) Delete(_ context.Context, _ string) error {
	return nil
}

func (r *stubRepo) DeleteByInstanceID(_ context.Context, _ int64) error {
	return nil
}

func (r *stubRepo) Close() error {
	r.closeCount.Add(1)

	return nil
}

func waitForListener(t *testing.T, s *Server, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.listenerMu.Lock()
		l := s.listener
		s.listenerMu.Unlock()
		if l != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("listener was not set within %s", timeout)
}

func waitForFindCount(t *testing.T, repo *stubRepo, minCount int32, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if repo.findCount.Load() >= minCount {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected repo.Find to be called at least %d times within %s, got %d",
		minCount, timeout, repo.findCount.Load())
}

func TestServer_StartClose_graceful(t *testing.T) {
	repo := &stubRepo{}
	s := NewServerAddr("127.0.0.1:0").
		WithRepository(repo).
		WithCacheRefreshInterval(0).
		WithLogger(nopLogger).
		MustBuild()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()

	waitForListener(t, s, 500*time.Millisecond)

	err := s.Close()
	if err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start() returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Start() did not return after Close()")
	}

	if repo.closeCount.Load() != 1 {
		t.Fatalf("expected repository.Close to be called once, got %d", repo.closeCount.Load())
	}
}

func TestServer_cacheUpdater_refreshesAndStops(t *testing.T) {
	repo := &stubRepo{}
	s := NewServerAddr("127.0.0.1:0").
		WithRepository(repo).
		WithCacheRefreshInterval(10 * time.Millisecond).
		WithLogger(nopLogger).
		MustBuild()

	done := make(chan struct{})
	go func() {
		s.cacheUpdater()
		close(done)
	}()

	waitForFindCount(t, repo, 2, 500*time.Millisecond)

	s.cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("cacheUpdater did not stop after cancel")
	}
}
