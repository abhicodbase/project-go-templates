// worker_pool.go — Bounded worker pool with graceful shutdown
package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Job represents work to be done
type Job struct {
	ID   int
	Data string
}

// Result represents the outcome of processing a job
type Result struct {
	JobID    int
	Output   string
	Duration time.Duration
	Err      error
}

// WorkerPool processes jobs with bounded concurrency
type WorkerPool struct {
	numWorkers int
	jobs       chan Job
	results    chan Result
	wg         sync.WaitGroup
}

func NewWorkerPool(numWorkers, bufSize int) *WorkerPool {
	return &WorkerPool{
		numWorkers: numWorkers,
		jobs:       make(chan Job, bufSize),
		results:    make(chan Result, bufSize),
	}
}

// Start launches worker goroutines
func (wp *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(ctx, i)
	}

	// Close results when all workers are done
	go func() {
		wp.wg.Wait()
		close(wp.results)
	}()
}

func (wp *WorkerPool) worker(ctx context.Context, id int) {
	defer wp.wg.Done()
	fmt.Printf("Worker %d started\n", id)

	for {
		select {
		case job, ok := <-wp.jobs:
			if !ok {
				fmt.Printf("Worker %d: jobs channel closed, exiting\n", id)
				return
			}
			start := time.Now()
			result := wp.process(ctx, job)
			result.Duration = time.Since(start)

			select {
			case wp.results <- result:
			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			fmt.Printf("Worker %d: context cancelled, exiting\n", id)
			return
		}
	}
}

func (wp *WorkerPool) process(ctx context.Context, job Job) Result {
	// Simulate variable work duration
	delay := time.Duration(rand.Intn(100)) * time.Millisecond
	select {
	case <-time.After(delay):
		return Result{
			JobID:  job.ID,
			Output: fmt.Sprintf("processed '%s' (worker took %v)", job.Data, delay),
		}
	case <-ctx.Done():
		return Result{JobID: job.ID, Err: ctx.Err()}
	}
}

// Submit sends a job to the pool
func (wp *WorkerPool) Submit(job Job) {
	wp.jobs <- job
}

// Close signals no more jobs
func (wp *WorkerPool) Close() {
	close(wp.jobs)
}

// Results returns the results channel for reading
func (wp *WorkerPool) Results() <-chan Result {
	return wp.results
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool := NewWorkerPool(3, 10)
	pool.Start(ctx)

	// Submit 20 jobs
	go func() {
		for i := 0; i < 20; i++ {
			pool.Submit(Job{
				ID:   i,
				Data: fmt.Sprintf("task-%d", i),
			})
		}
		pool.Close() // Signal no more jobs
	}()

	// Collect results
	var successCount, errorCount int
	for result := range pool.Results() {
		if result.Err != nil {
			fmt.Printf("Job %d FAILED: %v\n", result.JobID, result.Err)
			errorCount++
		} else {
			fmt.Printf("Job %d OK: %s [%v]\n", result.JobID, result.Output, result.Duration)
			successCount++
		}
	}

	fmt.Printf("\nDone! Success: %d, Errors: %d\n", successCount, errorCount)
}
