package main

import (
	"context"
	"fmt"
	"log"
	"main/fdd"

	//_ "net/http/pprof" // package for profiling mode
	"os"
	"os/signal"
	"runtime"
	"sync"

	"gopkg.in/yaml.v3"
)

// Often, reality is stranger than fiction.

const (
	//defPath string = "C:/Users/Zver/.vscode/p1/test"
	defPath            string = "."
	defConfigFileName  string = "config.yml"
	defResultFileName  string = "result.txt"
	defLoggerFileName  string = "main.log"
	defLoggerLevel     string = "debug" // "debug" or "info"
	defLoggerAddSource bool   = false
)

type mainOptions struct {
	Path            string `yaml:"Path"`
	configFileName  string
	ResultFileName  string `yaml:"File name for results"`
	LoggerFileName  string `yaml:"File name for logs"`
	LoggerLevel     string `yaml:"Logging (info/debug)"`
	LoggerAddSource bool   `yaml:"Adds source info in logs"`
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	// function for profiling mode
	/*
		go func() {
			fmt.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	*/

	// Default config
	config := &mainOptions{
		Path:            defPath,
		configFileName:  defConfigFileName,
		ResultFileName:  defResultFileName,
		LoggerFileName:  defLoggerFileName,
		LoggerLevel:     defLoggerLevel,
		LoggerAddSource: defLoggerAddSource,
	}
	// Overriding config from file
	overrideConfig(config)

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
	engine.Run(ctx, config.Path, callback)
	go progressMonitor(ctx, engine)
	wg.Wait()

	// Saving result to file
	saveResult(engine)
	// Printing total statistic
	printStatistic(engine)
}

func overrideConfig(config *mainOptions) {
	data, err := os.ReadFile(config.configFileName)
	if err != nil {
		fmt.Println(err)

		data, err := yaml.Marshal(&config)
		if err != nil {
			log.Fatalf("error: %v", err)
		}

		err = os.WriteFile(config.configFileName, data, 0644)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		fmt.Println("Application created config file and continues to work with default parameters")
	} else {
		err = yaml.Unmarshal(data, &config)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
	}
	fmt.Printf("---- CURRENT CONFIGURATION ----\n")
	fmt.Printf("Path:                       %s\n", config.Path)
	fmt.Printf("File name for results:      %s\n", config.ResultFileName)
	fmt.Printf("File name for logs:         %s\n", config.LoggerFileName)
	fmt.Printf("Logging level (info/debug): %s\n", config.LoggerLevel)
	fmt.Printf("Adds source info in logs:   %t\n", config.LoggerAddSource)
	fmt.Printf("-------------------------------\n")
}

func saveResult(engine fdd.SearchEngine) {
	/*er := os.Remove("result.txt")
	if er != nil {
		fmt.Println("Error of remove file: ", er)
	}*/
	resultFile, er := os.OpenFile("result.txt", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if er != nil {
		fmt.Println(er)
	}
	defer resultFile.Close()

	//result := engine.GetResult()
	
	for _, element := range engine.GetResult().List {
		fmt.Fprintf(resultFile, "     %d  {%d  %d  %d}\n", len(element.Paths), element.Size, element.Hash, element.Group)
		for _, path := range element.Paths {
			fmt.Fprintln(resultFile, path)
		}
	}
}
