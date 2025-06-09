package fdd

import (
	"container/list"
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

type queue struct {
	poolName   string
	mtx        sync.Mutex
	innerCall  chan struct{}
	done       atomic.Bool
	queueTasks *list.List
}

func newQueue(poolName string) *queue {
	item := queue{}
	item.poolName = poolName
	item.innerCall = make(chan struct{}, 1)
	item.queueTasks = list.New()
	return &item
}

func (item *queue) push(task *task) {
	item.mtx.Lock()
	item.queueTasks.PushBack(task)
	item.mtx.Unlock()
}

func (item *queue) pop() *task {
	item.mtx.Lock()
	defer item.mtx.Unlock()

	if item.queueTasks.Len() == 0 {
		return nil
	}
	elem := item.queueTasks.Front()
	item.queueTasks.Remove(elem)
	return elem.Value.(*task)
}

func (item *queue) getDone() bool {
	return item.done.Load()
}

func (item *queue) setDone() {
	item.done.Store(true)
}

// Create Queue (based on linked list)
// inpQueue and outQueue transforms Queue to the chanel
func inpQueue(poolName string, inp chan *task) *queue {
	queue := newQueue(poolName)
	queue.done.Store(false)
	go inpProcess(inp, queue)
	return queue
}

func inpProcess(inp chan *task, queue *queue) {
	logger.Debug("InpProcess of Queue - started.", slog.String("poolName", queue.poolName))

	for value := range inp {
		queue.push(value)

		select {
		case queue.innerCall <- struct{}{}:
		default:
		}
	}
	queue.setDone()
	close(queue.innerCall)

	logger.Debug("InpProcess of Queue - stoped.", slog.String("poolName", queue.poolName))
}

// Create output chanel for Queue
// inpQueue and outQueue transforms Queue to the chanel
func outQueue(ctx context.Context, queue *queue) chan *task {
	out := make(chan *task)
	go outProcess(ctx, queue, out)
	return out
}

func outProcess(ctx context.Context, queue *queue, out chan *task) {
	logger.Debug("OutProcess of Queue - started.", slog.String("poolName", queue.poolName))
	defer close(out)

	for {
		select {
		case <-ctx.Done():
			logger.Debug("OutProcess of Queue - cancelled.", slog.String("poolName", queue.poolName))
			return
		case <-queue.innerCall:
			for {
				task := queue.pop()
				if task != nil {
					select {
					case out <- task:
					case <-ctx.Done():
						logger.Debug("OutProcess of Queue - cancelled during task send.", slog.String("poolName", queue.poolName))
						return
					}
					continue
				}
				if !queue.getDone() {
					break
				}
				logger.Debug("OutProcess of Queue - stopped because queue is done and empty.", slog.String("poolName", queue.poolName))
				return
			}
		}
	}
}

func fetchQueue(ctx context.Context, inp chan *task, stat *metrics) chan *task {
	ch := counter(inp, stat.Fetch.Inp.add)
	ch = outQueue(ctx, inpQueue("fetchers", ch))
	ch = counter(ch, stat.Fetch.Out.add)
	return ch
}

func sizeQueue(ctx context.Context, inp chan *task, stat *metrics) chan *task {
	ch := counter(inp, stat.Size.Inp.add)
	ch = outQueue(ctx, inpQueue("sizers", ch))
	ch = counter(ch, stat.Size.Out.add)
	return ch
}

func hashQueue(ctx context.Context, inp chan *task, stat *metrics) chan *task {
	ch := counter(inp, stat.Hash.Inp.add)
	ch = outQueue(ctx, inpQueue("hashers", ch))
	ch = counter(ch, stat.Hash.Out.add)
	return ch
}

func matchQueue(ctx context.Context, inp chan *task, stat *metrics) chan *task {
	ch := counter(inp, stat.Match.Inp.add)
	ch = outQueue(ctx, inpQueue("matchers", ch))
	ch = counter(ch, stat.Match.Out.add)
	return ch
}

func packQueue(ctx context.Context, inp chan *task, stat *metrics) chan *task {
	ch := counter(inp, stat.Pack.Inp.add)
	ch = outQueue(ctx, inpQueue("packer", ch))
	ch = counter(ch, stat.Pack.Out.add)
	return ch
}
