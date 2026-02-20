package worker

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
)

// Task represents a unit of work to be processed by the pool.
type Task[T any, R any] struct {
	Input  T
	Result R
	Err    error
}

// ProcessFunc is the function signature for processing a single task.
type ProcessFunc[T any, R any] func(ctx context.Context, input T) (R, error)

// Pool is a generic worker pool with configurable concurrency.
type Pool[T any, R any] struct {
	workers int
	process ProcessFunc[T, R]
}

// NewPool creates a new worker pool.
func NewPool[T any, R any](workers int, fn ProcessFunc[T, R]) *Pool[T, R] {
	if workers < 1 {
		workers = 1
	}
	return &Pool[T, R]{
		workers: workers,
		process: fn,
	}
}

// Execute runs all inputs through the worker pool and returns results.
// Supports context cancellation and graceful shutdown.
func (p *Pool[T, R]) Execute(ctx context.Context, inputs []T) []Task[T, R] {
	results := make([]Task[T, R], len(inputs))
	inputCh := make(chan int, len(inputs))

	var wg sync.WaitGroup

	// Start workers.
	for w := 0; w < p.workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case idx, ok := <-inputCh:
					if !ok {
						return
					}
					result, err := p.process(ctx, inputs[idx])
					results[idx] = Task[T, R]{
						Input:  inputs[idx],
						Result: result,
						Err:    err,
					}
					if err != nil {
						log.Error().Err(err).Int("worker", workerID).Int("index", idx).Msg("Task failed")
					}
				}
			}
		}(w)
	}

	// Send inputs.
	for i := range inputs {
		select {
		case <-ctx.Done():
			break
		case inputCh <- i:
		}
	}
	close(inputCh)

	// Wait for all workers to finish.
	wg.Wait()
	return results
}

// Batch splits inputs into batches and processes each batch.
func Batch[T any](items []T, batchSize int) [][]T {
	if batchSize <= 0 {
		batchSize = 1
	}
	var batches [][]T
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[i:end])
	}
	return batches
}
