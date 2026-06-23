package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/roncofaber/chemres/internal/resolver"
)

const TTL = 10 * time.Minute

type Snapshot struct {
	Done     int
	Total    int
	Finished bool
	Results  []resolver.CompoundResult
	Err      error
}

type Job struct {
	mu       sync.Mutex
	total    int
	done     int
	finished bool
	results  []resolver.CompoundResult
	err      error
	created  time.Time
	Ctx      context.Context
	cancel   context.CancelFunc
}

func (j *Job) Incr() {
	j.mu.Lock()
	j.done++
	j.mu.Unlock()
}

func (j *Job) Finish(results []resolver.CompoundResult, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.results = results
	j.err = err
	j.finished = true
}

func (j *Job) Snapshot() Snapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Snapshot{
		Done:     j.done,
		Total:    j.total,
		Finished: j.finished,
		Results:  j.results,
		Err:      j.err,
	}
}

type Store struct {
	jobs sync.Map
}

func NewStore() *Store {
	s := &Store{}
	// Sweep goroutine runs for process lifetime; Store is a singleton.
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			s.sweepOnce()
		}
	}()
	return s
}

func (s *Store) New(total int) (string, *Job) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("jobs: rand.Read: %v", err))
	}
	id := hex.EncodeToString(b)
	ctx, cancel := context.WithCancel(context.Background())
	job := &Job{
		total:   total,
		created: time.Now(),
		Ctx:     ctx,
		cancel:  cancel,
	}
	s.jobs.Store(id, job)
	return id, job
}

func (s *Store) Get(id string) (*Job, bool) {
	v, ok := s.jobs.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*Job), true
}

func (s *Store) sweepOnce() {
	now := time.Now()
	s.jobs.Range(func(k, v interface{}) bool {
		job := v.(*Job)
		job.mu.Lock()
		expired := now.Sub(job.created) > TTL
		if expired {
			job.cancel()
		}
		job.mu.Unlock()
		if expired {
			s.jobs.Delete(k)
		}
		return true
	})
}
