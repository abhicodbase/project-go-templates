# Concurrency Patterns in Go

## 1. Worker Pool

Bounded concurrency — process N jobs with M workers.

```go
package main

import (
    "context"
    "fmt"
    "sync"
)

type Job struct {
    ID   int
    Data string
}

type Result struct {
    JobID  int
    Output string
    Err    error
}

// WorkerPool runs jobs with bounded concurrency
func WorkerPool(ctx context.Context, numWorkers int, jobs <-chan Job) <-chan Result {
    results := make(chan Result, numWorkers)
    var wg sync.WaitGroup

    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            for {
                select {
                case job, ok := <-jobs:
                    if !ok {
                        return // jobs channel closed
                    }
                    result := process(ctx, job)
                    results <- result
                case <-ctx.Done():
                    return // context cancelled
                }
            }
        }(i)
    }

    // Close results when all workers done
    go func() {
        wg.Wait()
        close(results)
    }()

    return results
}

func process(ctx context.Context, job Job) Result {
    // Simulate work
    return Result{JobID: job.ID, Output: "processed: " + job.Data}
}

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Send 100 jobs
    jobs := make(chan Job, 100)
    for i := 0; i < 100; i++ {
        jobs <- Job{ID: i, Data: fmt.Sprintf("data-%d", i)}
    }
    close(jobs)

    // Process with 5 workers
    results := WorkerPool(ctx, 5, jobs)
    for result := range results {
        if result.Err != nil {
            fmt.Printf("job %d failed: %v\n", result.JobID, result.Err)
        } else {
            fmt.Printf("job %d: %s\n", result.JobID, result.Output)
        }
    }
}
```

---

## 2. Pipeline Pattern

Connect stages where output of one is input to the next.

```go
// Stage 1: Generate numbers
func generate(ctx context.Context, nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for _, n := range nums {
            select {
            case out <- n:
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}

// Stage 2: Square numbers
func square(ctx context.Context, in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for n := range in {
            select {
            case out <- n * n:
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}

// Stage 3: Filter even numbers
func filterEven(ctx context.Context, in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for n := range in {
            if n%2 == 0 {
                select {
                case out <- n:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()
    return out
}

// Compose the pipeline
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Pipeline: generate → square → filter even
    nums := generate(ctx, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
    squares := square(ctx, nums)
    evens := filterEven(ctx, squares)

    for n := range evens {
        fmt.Println(n) // 4, 16, 36, 64, 100
    }
}
```

---

## 3. Fan-Out / Fan-In

**Fan-Out**: Distribute work across multiple goroutines
**Fan-In**: Merge multiple channels into one

```go
// Fan-Out: distribute input to multiple workers
func fanOut(ctx context.Context, in <-chan Job, numWorkers int) []<-chan Result {
    channels := make([]<-chan Result, numWorkers)
    for i := 0; i < numWorkers; i++ {
        channels[i] = worker(ctx, in) // all workers read from same input
    }
    return channels
}

// Fan-In: merge multiple channels into one
func fanIn(ctx context.Context, channels ...<-chan Result) <-chan Result {
    merged := make(chan Result)
    var wg sync.WaitGroup

    forward := func(ch <-chan Result) {
        defer wg.Done()
        for result := range ch {
            select {
            case merged <- result:
            case <-ctx.Done():
                return
            }
        }
    }

    wg.Add(len(channels))
    for _, ch := range channels {
        go forward(ch)
    }

    go func() {
        wg.Wait()
        close(merged)
    }()

    return merged
}
```

---

## 4. Semaphore (Limit Concurrency)

```go
// Use buffered channel as semaphore
type Semaphore chan struct{}

func NewSemaphore(n int) Semaphore {
    return make(Semaphore, n)
}

func (s Semaphore) Acquire() { s <- struct{}{} }
func (s Semaphore) Release() { <-s }

// Limit to 10 concurrent HTTP requests
sem := NewSemaphore(10)

for _, url := range urls {
    sem.Acquire()
    go func(url string) {
        defer sem.Release()
        fetch(url)
    }(url)
}
```

---

## 5. errgroup — Concurrent tasks with error handling

```go
import "golang.org/x/sync/errgroup"

func fetchAll(ctx context.Context, ids []string) ([]Hotel, error) {
    g, ctx := errgroup.WithContext(ctx)
    results := make([]Hotel, len(ids))

    for i, id := range ids {
        i, id := i, id // capture loop variables
        g.Go(func() error {
            hotel, err := fetchHotel(ctx, id)
            if err != nil {
                return fmt.Errorf("fetch hotel %s: %w", id, err)
            }
            results[i] = hotel
            return nil
        })
    }

    if err := g.Wait(); err != nil {
        return nil, err // returns first error, cancels context
    }
    return results, nil
}
```

---

## Interview Q&A

**Q: How do you implement a worker pool in Go?**
> A: I'd use a jobs channel and N goroutines reading from it. The key points: (1) close the jobs channel when all work is submitted — workers exit cleanly when ranging over a closed channel; (2) use a WaitGroup to know when all workers are done; (3) close the results channel after all workers exit; (4) handle context cancellation in each worker so we can stop gracefully. The buffered channels for results prevent workers from blocking on slow consumers.

**Q: What is the difference between fan-out and fan-in?**
> A: Fan-out distributes work from one channel to multiple goroutines — useful when you want to parallelize processing of a stream of work. Fan-in merges multiple channels into one — useful when you have multiple producers and one consumer. They're often used together: fan-out to distribute work, each worker produces results, fan-in to collect all results. The key implementation detail for fan-in: you need a goroutine per input channel forwarding to the merged output, and a WaitGroup to close the merged channel when all inputs are exhausted.
