/*
File Duplicate Detector Package
*/
package fdd

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

var foldersCount sync.WaitGroup
var workerPools sync.WaitGroup
var logger *slog.Logger

type KeyCtxLogger struct{}

/*
	type engineOptions struct {
		inpChan      chan *task
		foldersCount atomic.Int64
		workerPools  sync.WaitGroup
		logger       *slog.Logger
	}

	type engineVariables struct {
		inpChan      chan *task
		foldersCount atomic.Int64
		workerPools  sync.WaitGroup
		logger       *slog.Logger
	}
*/
func loggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(KeyCtxLogger{}).(*slog.Logger); ok {
		return logger
	}

	return slog.Default()
}

/*
	func ErrAttr(err error) slog.Attr {
		if err == nil {
			return slog.String("error", "nil")
		}

		return slog.String("error", err.Error())
	}
*/

type Callback func()

type SearchEngine interface {
	Run(ctx context.Context, path string, callback Callback)
	GetProgress() []byte
	GetResult() *Result
}

func GetEngine() SearchEngine {
	return newSearchEngine()
}

type searchEngine struct {
	//vars *engineVariables
	callback Callback
	result   *Result
	metrics  *metrics
}

func newSearchEngine() *searchEngine {
	se := &searchEngine{}
	//se.vars = &engineVariables{}
	return se
}

func (item *searchEngine) Run(ctx context.Context, rootPath string, callback Callback) {
	item.callback = callback
	logger = loggerFromContext(ctx)
	item.init()
	go item.runPipeline(ctx, rootPath)
}

func (item *searchEngine) GetProgress() []byte {
	item.metrics.Duration = time.Since(item.metrics.StartTime)
	jsonData, err := json.Marshal(item.metrics)
	if err != nil {
		logger.Info("Marshalling error",
			slog.String("method", "json.Marshal"),
			slog.String("error", err.Error()))
	}
	return jsonData
}

func (item *searchEngine) GetResult() *Result {
	return item.result
}

func (item *searchEngine) init() {
	item.metrics = &metrics{}
	item.metrics.StartTime = time.Now()
}

func (item *searchEngine) runPipeline(ctx context.Context, rootPath string) {
	predResult := newPredResult()

	for task := range item.pipeline(ctx, rootPath) {
		pathList, detected := predResult.List[task.key]
		if detected {
			predResult.List[task.key] = append(pathList, task.info.path)
		} else {
			pathList := make([]string, 1)
			pathList[0] = task.info.path
			predResult.List[task.key] = pathList
		}
	}
	item.result = newResult(predResult)

	workerPools.Wait()
	item.callback()
}

func (item *searchEngine) pipeline(ctx context.Context, rootPath string) chan *task {
	rec := make(chan *task)

	ch := fileGenerator(rootPath, 3, rec, fetchQueue(ctx, rec, item.metrics))
	ch = runPool(&sizer{}, 1, sizeQueue(ctx, ch, item.metrics), newCheckList())
	ch = runPool(&hasher{}, 6, hashQueue(ctx, ch, item.metrics), newCheckList())
	ch = runPool(&matcher{}, 7, matchQueue(ctx, ch, item.metrics), newCheckList())
	ch = resultQueue(ctx, ch, item.metrics)
	return ch
}
