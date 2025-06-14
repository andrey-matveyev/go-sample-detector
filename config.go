package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	defPath            string = "."
	defConfigFileName  string = "fdd-config.yml"
	defResultFileName  string = "fdd-result.txt"
	defLoggerFileName  string = "fdd-output.log"
	defLoggerLevel     string = "debug" // "debug" or "info"
	defLoggerAddSource bool   = false
)

type config struct {
	RootPath        string `yaml:"Root Path"`
	configFileName  string
	ResultFileName  string `yaml:"File name for results"`
	LoggerFileName  string `yaml:"File name for logs"`
	LoggerLevel     string `yaml:"Logging level (info/debug)"`
	LoggerAddSource bool   `yaml:"Adds source info in logs"`
}

func newConfig() *config {
	item := &config{
		RootPath:        defPath,
		configFileName:  defConfigFileName,
		ResultFileName:  defResultFileName,
		LoggerFileName:  defLoggerFileName,
		LoggerLevel:     defLoggerLevel,
		LoggerAddSource: defLoggerAddSource,
	}
	item.overrideConfig()
	return item
}

func (item *config) overrideConfig() {
	data, err := os.ReadFile(item.configFileName)
	if err != nil {
		fmt.Println(err)

		data, err := yaml.Marshal(&item)
		if err != nil {
			fmt.Printf("error: %v", err)
			os.Exit(1)
		}

		err = os.WriteFile(item.configFileName, data, 0644)
		if err != nil {
			fmt.Printf("error: %v", err)
			os.Exit(1)
		}
		fmt.Println("Application created config file and continues to work with default parameters")
	} else {
		err = yaml.Unmarshal(data, &item)
		if err != nil {
			fmt.Printf("error: %v", err)
			os.Exit(1)
		}
	}
	fmt.Printf("---- CURRENT CONFIGURATION ----\n")
	fmt.Printf("Root Path:                  %s\n", item.RootPath)
	fmt.Printf("File name for results:      %s\n", item.ResultFileName)
	fmt.Printf("File name for logs:         %s\n", item.LoggerFileName)
	fmt.Printf("Logging level (info/debug): %s\n", item.LoggerLevel)
	fmt.Printf("Adds source info in logs:   %t\n", item.LoggerAddSource)
	fmt.Printf("-------------------------------\n")
}
