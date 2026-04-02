package render

import (
	"context"
	"log"
	"sync"

	"video-editor/internal/queue"
)

type WorkerPool struct {
	queue   queue.Queue
	workers int
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

func NewWorkerPool(q queue.Queue, workerCount int) *WorkerPool {
	return &WorkerPool{
		queue:   q,
		workers: workerCount,
	}
}

func (wp *WorkerPool) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	wp.cancel = cancel

	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(ctx, i)
	}

	log.Printf("Worker pool started with %d workers", wp.workers)
}

func (wp *WorkerPool) Stop() {
	if wp.cancel != nil {
		wp.cancel()
	}
	wp.wg.Wait()
	log.Printf("Worker pool stopped")
}

func (wp *WorkerPool) worker(ctx context.Context, id int) {
	defer wp.wg.Done()
	log.Printf("Worker %d started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d stopping", id)
			return
		case job := <-wp.queue.Pop():
			log.Printf("Worker %d processing job %s", id, job.ID)
			if err := job.Execute(); err != nil {
				log.Printf("Worker %d job %s failed: %v", id, job.ID, err)
			} else {
				log.Printf("Worker %d job %s complete", id, job.ID)
			}
		}
	}
}
