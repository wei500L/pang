package scenes

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// DefaultQueueCapacity bounds queued-but-not-started scene jobs per process.
const DefaultQueueCapacity = 32

// Job is one scene to generate. The owner is resolved from the stored row when
// the job starts, so the queue never needs owner data at enqueue time.
type Job struct {
	SceneID string
}

// Worker is a bounded asynchronous scene-generation queue. Enqueue never
// blocks: when the queue is full it returns ErrQueueFull so callers can answer
// 429/503. Concurrency is capped by the configured semaphore; POST generate
// only enqueues, so the HTTP handler never blocks on provider latency.
type Worker struct {
	service *Service
	queue   chan Job
	sem     chan struct{}
	logger  *slog.Logger
	timeout time.Duration
	done    chan struct{}
}

// NewWorker builds the bounded queue. concurrency must be >= 1.
func NewWorker(service *Service, concurrency, queueCapacity int, timeout time.Duration, logger *slog.Logger) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	if queueCapacity < 1 {
		queueCapacity = DefaultQueueCapacity
	}
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	return &Worker{
		service: service,
		queue:   make(chan Job, queueCapacity),
		sem:     make(chan struct{}, concurrency),
		logger:  logger,
		timeout: timeout,
		done:    make(chan struct{}),
	}
}

// Enqueue places a scene into the bounded queue. Non-blocking; full queue
// returns ErrQueueFull. Duplicate starts are prevented by the store CAS, not
// by the queue.
func (w *Worker) Enqueue(sceneID string) error {
	if sceneID == "" {
		return &Error{Message: "scene id is required"}
	}
	select {
	case w.queue <- Job{SceneID: sceneID}:
		return nil
	default:
		return ErrQueueFull
	}
}

// Run consumes the queue until ctx is cancelled. It returns after all in-flight
// jobs finish, so a graceful shutdown does not leave half-written files. The
// concurrency slot is acquired in its own select so a full semaphore can never
// block the shutdown path.
func (w *Worker) Run(ctx context.Context) {
	defer close(w.done)
	var wait sync.WaitGroup
	defer wait.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case w.sem <- struct{}{}:
		}
		select {
		case <-ctx.Done():
			<-w.sem
			return
		case job := <-w.queue:
			wait.Add(1)
			go func(job Job) {
				defer wait.Done()
				defer func() { <-w.sem }()
				w.runOne(ctx, job)
			}(job)
		}
	}
}

// Done returns a channel closed when Run has returned.
func (w *Worker) Done() <-chan struct{} { return w.done }

func (w *Worker) runOne(ctx context.Context, job Job) {
	project, err := w.service.store.GetUnscoped(job.SceneID)
	if err != nil {
		w.logger.Warn("scene_job_missing", "scene_id", job.SceneID, "error", err)
		return
	}
	// The job context derives from the worker context so a graceful shutdown
	// cancels in-flight provider requests, and additionally bounds one whole
	// job (brief composition + image generation) by the configured timeout.
	jobCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	w.service.runGeneration(jobCtx, project)
}
