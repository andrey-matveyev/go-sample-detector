// File Duplicate Detector Package
package fdd

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// https://stackoverflow.com/questions/53314863/fastest-algorithm-to-detect-duplicate-files

type SearchEngine interface {
	Run(ctx context.Context, rootPath string, callback func())
	GetProgress() []byte
	GetResult() *Result
}

func GetEngine() SearchEngine {
	return &searchEngine{}
}

type searchEngine struct {
	rootPath  string
	callback  func()
	poolCount sync.WaitGroup
	result    *Result
	metrics   *metrics
}

func (item *searchEngine) Run(ctx context.Context, rootPath string, callback func()) {
	item.rootPath = rootPath
	item.callback = callback
	item.metrics = &metrics{}
	item.metrics.StartTime = time.Now()
	go item.runPipeline(ctx)
}

func (item *searchEngine) runPipeline(ctx context.Context) {
	predResult := newPredResult()

	for task := range item.pipeline(ctx) {
		pathList, detected := predResult.List[task.key]
		if detected {
			predResult.List[task.key] = append(pathList, task.info.path)
		} else {
			pathList := make([]string, 1)
			pathList[0] = task.info.path
			predResult.List[task.key] = pathList
		}
	}
	item.result = result(predResult)

	item.poolCount.Wait()

	item.callback()
}

func (item *searchEngine) pipeline(ctx context.Context) chan *task {
	rec := make(chan *task)

	out := fileGenerator(item.rootPath, 4, rec, fetchQueue(ctx, rec, item.metrics), item)
	out = runPool(&sizer{}, 1, sizeQueue(ctx, out, item.metrics), newCheckList(), item)
	out = runPool(&hasher{}, 6, hashQueue(ctx, out, item.metrics), newCheckList(), item)
	out = runPool(&matcher{}, 8, matchQueue(ctx, out, item.metrics), newCheckList(), item)
	out = resultQueue(ctx, out, item.metrics)
	return out
}

func (item *searchEngine) GetProgress() []byte {
	item.metrics.Duration = time.Since(item.metrics.StartTime)
	jsonData, err := json.Marshal(item.metrics)
	if err != nil {
		slog.Info("Marshalling error",
			slog.String("method", "json.Marshal"),
			slog.String("error", err.Error()))
	}
	return jsonData
}

func (item *searchEngine) GetResult() *Result {
	return item.result
}
