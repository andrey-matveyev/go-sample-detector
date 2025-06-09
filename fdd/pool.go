package fdd

import (
	"fmt"
	"log/slog"
	"sync"
)

func runPool(runWorker worker, amt int, inp, out chan *task, checker checker) {
	var workers sync.WaitGroup

	workerPools.Add(1)
	for range amt {
		workers.Add(1)
		go func() {
			defer workers.Done()
			runWorker.run(inp, out, checker)
		}()
	}
	logger.Debug("Worker-pool - started.", slog.String("workerType", fmt.Sprintf("%T", runWorker)))

	go func(currentWorker worker, outChan chan *task) {
		workers.Wait()
		close(outChan)
		workerPools.Done()

		logger.Debug("Worker-pool - stoped.", slog.String("workerType", fmt.Sprintf("%T", runWorker)))
	}(runWorker, out)
}

func fetch(amt int, inp, outFolder chan *task) (out chan *task) {
	out = make(chan *task)
	runPool(fetcher{outFolder: outFolder}, amt, inp, out, nil)
	return out
}

func size(amt int, inp chan *task) (out chan *task) {
	out = make(chan *task)
	runPool(sizer{}, amt, inp, out, newCheckList())
	return out
}

func hash(amt int, inp chan *task) (out chan *task) {
	out = make(chan *task)
	runPool(hasher{}, amt, inp, out, newCheckList())
	return out
}

func match(amt int, inp chan *task) (out chan *task) {
	out = make(chan *task)
	runPool(matcher{}, amt, inp, out, newCheckList())
	return out
}

// Create a global input channel for the fetcher working recursively
func start(path string) (out chan *task) {
	out = make(chan *task)
	foldersCount.Add(1)
	go func() {
		out <- newTask(key{}, info{path: path})
	}()
	go func() {
		foldersCount.Wait()
		close(out)
	}()
	return out
}

/*
func watch(ctx context.Context, queue *queue) *queue {
	go watcher(ctx, queue)
	return queue
}

func watcher(ctx context.Context, queue *queue) {
	for {
		select {
		case <-ctx.Done():
			logger.Debug("Watcher cancelled.")
			return
		default:
			count := foldersCount.Load()
			if count == 0 {
				queue.setDone()
				logger.Debug("All folders are processed. Watcher - stoped.")
				return
			}
			runtime.Gosched()
		}
	}
}
*/
