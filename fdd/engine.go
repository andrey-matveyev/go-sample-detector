// File Duplicate Detector Package
package fdd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// https://stackoverflow.com/questions/53314863/fastest-algorithm-to-detect-duplicate-files

const (
	defPath            string = "."
	defOptionsFileName string = "fdd-config.yml"
	defResultFileName  string = "fdd-result.txt"
	defLoggerFileName  string = "fdd-output.log"
	defLoggerLevel     string = "debug" // "debug" or "info"
	defLoggerAddSource bool   = false
)

type fddOptions struct {
	RootPath        string `yaml:"Root Path"`
	OptionsFileName string
	ResultFileName  string `yaml:"File name for results"`
	LoggerFileName  string `yaml:"File name for logs"`
	LoggerLevel     string `yaml:"Logging level (info/debug)"`
	LoggerAddSource bool   `yaml:"Adds source info in logs"`
}

type SearchEngine interface {
	Run(ctx context.Context, callback func())
	GetProgress() []byte
	GetResult() *Result
}

func GetEngine() SearchEngine {
	return newSearchEngine()
}

type searchEngine struct {
	options   *fddOptions
	defLogger *slog.Logger
	logger    *slog.Logger
	logFile   *os.File
	callback  func()
	poolCount sync.WaitGroup
	result    *Result
	metrics   *metrics
}

func newSearchEngine() *searchEngine {
	item := &searchEngine{}
	item.options = &fddOptions{
		RootPath:        defPath,
		OptionsFileName: defOptionsFileName,
		ResultFileName:  defResultFileName,
		LoggerFileName:  defLoggerFileName,
		LoggerLevel:     defLoggerLevel,
		LoggerAddSource: defLoggerAddSource,
	}
	return item
}

func (item *searchEngine) Run(ctx context.Context, callback func()) {
	item.init(callback)
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
	item.result = newResult(predResult)

	item.poolCount.Wait()
	item.logFile.Close()
	slog.SetDefault(item.defLogger)
	item.callback()
}

func (item *searchEngine) pipeline(ctx context.Context) chan *task {
	rec := make(chan *task)

	out := fileGenerator(item.options.RootPath, 4, rec, fetchQueue(ctx, rec, item.metrics), item)
	out = runPool(&sizer{}, 1, sizeQueue(ctx, out, item.metrics), newCheckList(), item)
	out = runPool(&hasher{}, 6, hashQueue(ctx, out, item.metrics), newCheckList(), item)
	out = runPool(&matcher{}, 8, matchQueue(ctx, out, item.metrics), newCheckList(), item)
	out = resultQueue(ctx, out, item.metrics)
	return out
}

func (item *searchEngine) init(callback func()) {
	item.callback = callback

	item.overrideOptions()

	item.logFile = newLogFile(item.options.LoggerFileName)
	item.defLogger = slog.Default()
	item.logger = newLogger(
		withLevel(item.options.LoggerLevel),
		withAddSource(item.options.LoggerAddSource),
		withLogFile(item.logFile),
		withSetDefault(true),
	)

	item.metrics = &metrics{}
	item.metrics.StartTime = time.Now()
}

func (item *searchEngine) overrideOptions() {
	data, err := os.ReadFile(item.options.OptionsFileName)
	if err != nil {
		fmt.Println(err)

		data, err := yaml.Marshal(&item.options)
		if err != nil {
			log.Fatalf("error: %v", err)
		}

		err = os.WriteFile(item.options.OptionsFileName, data, 0644)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		fmt.Println("Application created config file and continues to work with default parameters")
	} else {
		err = yaml.Unmarshal(data, &item.options)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
	}
	fmt.Printf("---- CURRENT CONFIGURATION ----\n")
	fmt.Printf("Root Path:                  %s\n", item.options.RootPath)
	fmt.Printf("File name for results:      %s\n", item.options.ResultFileName)
	fmt.Printf("File name for logs:         %s\n", item.options.LoggerFileName)
	fmt.Printf("Logging level (info/debug): %s\n", item.options.LoggerLevel)
	fmt.Printf("Adds source info in logs:   %t\n", item.options.LoggerAddSource)
	fmt.Printf("-------------------------------\n")
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
