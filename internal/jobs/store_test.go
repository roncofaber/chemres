package jobs

import (
	"fmt"
	"testing"
	"time"

	"github.com/roncofaber/chemres/internal/resolver"
)

func newTestStore() *Store { return &Store{} }

func TestStore_New(t *testing.T) {
	s := newTestStore()
	id, job := s.New(10)
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	got, ok := s.Get(id)
	if !ok || got != job {
		t.Fatal("job not retrievable after creation")
	}
}

func TestStore_Get_Missing(t *testing.T) {
	s := newTestStore()
	_, ok := s.Get("does-not-exist")
	if ok {
		t.Fatal("expected false for unknown ID")
	}
}

func TestJob_Incr(t *testing.T) {
	s := newTestStore()
	_, job := s.New(5)
	job.Incr()
	job.Incr()
	job.Incr()
	snap := job.Snapshot()
	if snap.Done != 3 {
		t.Errorf("done: got %d, want 3", snap.Done)
	}
	if snap.Total != 5 {
		t.Errorf("total: got %d, want 5", snap.Total)
	}
}

func TestJob_Finish_Success(t *testing.T) {
	s := newTestStore()
	_, job := s.New(2)
	results := []resolver.CompoundResult{{IUPAC: "water"}}
	job.Finish(results, nil)
	snap := job.Snapshot()
	if !snap.Finished {
		t.Fatal("expected finished=true")
	}
	if len(snap.Results) != 1 || snap.Results[0].IUPAC != "water" {
		t.Errorf("unexpected results: %v", snap.Results)
	}
	if snap.Err != nil {
		t.Errorf("unexpected err: %v", snap.Err)
	}
}

func TestJob_Finish_Error(t *testing.T) {
	s := newTestStore()
	_, job := s.New(1)
	job.Finish(nil, errSentinel)
	snap := job.Snapshot()
	if !snap.Finished {
		t.Fatal("expected finished=true")
	}
	if snap.Err != errSentinel {
		t.Errorf("err: got %v, want sentinel", snap.Err)
	}
}

var errSentinel = fmt.Errorf("sentinel error")

func TestStore_SweepExpired(t *testing.T) {
	s := newTestStore()
	id, job := s.New(1)

	job.mu.Lock()
	job.created = time.Now().Add(-11 * time.Minute)
	job.mu.Unlock()

	s.sweepOnce()

	_, ok := s.Get(id)
	if ok {
		t.Fatal("expected expired job to be removed")
	}
	select {
	case <-job.Ctx.Done():
	default:
		t.Fatal("expected job context to be cancelled after sweep")
	}
}

func TestStore_SweepKeepsFresh(t *testing.T) {
	s := newTestStore()
	id, _ := s.New(1)
	s.sweepOnce()
	_, ok := s.Get(id)
	if !ok {
		t.Fatal("fresh job should not be swept")
	}
}
