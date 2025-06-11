/*
File Duplicate Detector Package
*/
package fdd

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"
)

// https://stackoverflow.com/questions/53314863/fastest-algorithm-to-detect-duplicate-files

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
	defLogger    *slog.Logger
	logger    *slog.Logger
	logFile   *os.File
	callback  Callback
	poolCount sync.WaitGroup
	result    *Result
	metrics   *metrics
}

func newSearchEngine() *searchEngine {
	se := &searchEngine{}
	//se.vars = &engineVariables{}
	return se
}

func (item *searchEngine) Run(ctx context.Context, rootPath string, callback Callback) {
	item.callback = callback
	//logger = loggerFromContext(ctx)
	// Preparing file for logging
	item.logFile = newLogFile("main.log")
	//defer logFile.Close()
	// Creating logger and including to context
	item.defLogger = slog.Default()
	item.logger = NewLogger(
		WithLevel("debug"),
		WithAddSource(false),
		WithLogFile(item.logFile),
		WithSetDefault(true),
	)

	item.init()
	go item.runPipeline(ctx, rootPath)
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

	item.poolCount.Wait()
	item.logFile.Close()
	slog.SetDefault(item.defLogger)
	item.callback()
}

func (item *searchEngine) pipeline(ctx context.Context, rootPath string) chan *task {
	rec := make(chan *task)

	out := fileGenerator(rootPath, 4, rec, fetchQueue(ctx, rec, item.metrics), item)
	out = runPool(&sizer{}, 1, sizeQueue(ctx, out, item.metrics), newCheckList(), item)
	out = runPool(&hasher{}, 6, hashQueue(ctx, out, item.metrics), newCheckList(), item)
	out = runPool(&matcher{}, 8, matchQueue(ctx, out, item.metrics), newCheckList(), item)
	out = resultQueue(ctx, out, item.metrics)
	return out
}
