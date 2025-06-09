package fdd

import (
	"fmt"
	"sync/atomic"
	"time"
)

type addFunc func(size int64)

func counter(inp chan *task, adder addFunc) chan *task {
	out := make(chan *task)
	go func() {
		for task := range inp {
			out <- task
			adder(task.key.size)
		}
		close(out)
	}()
	return out
}

type metrics struct {
	StartTime time.Time     `json:"start"`
	Duration  time.Duration `json:"duration"`
	Fetch     queueStat     `json:"fetch"`
	Size      queueStat     `json:"size"`
	Hash      queueStat     `json:"hash"`
	Match     queueStat     `json:"match"`
	Pack      queueStat     `json:"pack"`
}

type queueStat struct {
	Inp statistic `json:"inpQueue"`
	Out statistic `json:"outQueue"`
}

type statistic struct {
	Count customInt64 `json:"count"`
	Size  customInt64 `json:"size"`
}

func (item *statistic) add(size int64) {
	item.Count.Add(1)
	item.Size.Add(size)
}

type customInt64 struct {
	atomic.Int64
}

func (item *customInt64) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `%d`, item.Load()), nil
}
