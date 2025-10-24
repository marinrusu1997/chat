package concurrency

import (
	"context"
	"sync"
)

// TaskResult holds the outcome of a single task
type TaskResult[T any] struct {
	Result T
	Err    error
}

type Task[T any] = func() (T, error)

// AllSettled runs all tasks concurrently and returns a slice of TaskResult[T]
func AllSettled[T any](ctx context.Context, tasks []Task[T]) []TaskResult[T] {
	results := make([]TaskResult[T], len(tasks))
	if len(tasks) == 0 {
		return results
	}

	var wg sync.WaitGroup
	wg.Add(len(tasks))

	for i, task := range tasks {
		i, task := i, task // capture loop variables
		go func() {
			defer wg.Done()

			select {
			case <-ctx.Done(): // Task canceled due to context
				results[i] = TaskResult[T]{Err: ctx.Err()}
			default:
				res, err := task()
				results[i] = TaskResult[T]{Result: res, Err: err}
			}
		}()
	}

	wg.Wait()
	return results
}

func RunAll(tasks []func()) {
	if len(tasks) == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(tasks))

	for _, task := range tasks {
		task := task
		go func() {
			task()
			wg.Done()
		}()
	}

	wg.Wait()
}
