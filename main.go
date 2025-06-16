package main

import (
	"context"
	"fmt"
	"log/slog"
	"main/fdd"

	//_ "net/http/pprof" // package for profiling mode
	"os"
	"os/signal"
	"runtime"
	"sync"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	// function for profiling mode
	/*
		go func() {
			fmt.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	*/
	config := newConfig()

	oldLogger := slog.Default()

	logFile := newLogFile(config.LoggerFileName)
	newLogger(
		withLevel(config.LoggerLevel),
		withAddSource(config.LoggerAddSource),
		withLogFile(logFile),
		withSetDefault(true),
	)

	// Creating context with Cancel
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	// Preparing cancel-mechanism (over signal <Ctrl+C>)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		fmt.Println("Canselling... ")
		fmt.Println("The application needs to close all resources and save the current result.")
		fmt.Println("Please wait...")
		cancel()
	}()

	engine := fdd.GetEngine()
	// Preparing callback function (event about all tasks completed)
	var wg sync.WaitGroup
	callback := func() {
		wg.Done()
	}
	// Running main work and progress-monitor
	wg.Add(1)
	engine.Run(ctx, config.RootPath, callback)
	go progressMonitor(ctx, engine)
	wg.Wait()

	// Saving result to file
	saveResult(engine, config.ResultFileName)
	// Saving logs
	logFile.Close()
	// Returning default logger
	slog.SetDefault(oldLogger)
	// Printing total statistic
	printStatistic(engine, config)
}

func saveResult(engine fdd.SearchEngine, fileName string) {
	resultFile, er := os.OpenFile(fileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if er != nil {
		fmt.Println(er)
	}
	defer resultFile.Close()

	for _, element := range engine.GetResult().List {
		fmt.Fprintf(resultFile, "     %d  {%d  %d  %d}\n", len(element.Paths), element.Size, element.Hash, element.Group)
		for _, path := range element.Paths {
			fmt.Fprintln(resultFile, path)
		}
	}
}
