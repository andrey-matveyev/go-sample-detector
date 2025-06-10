package main

import (
	"context"
	"encoding/json"
	"fmt"
	"main/fdd"
	"runtime"
	"time"
)

func progressMonitor(ctx context.Context, engine fdd.SearchEngine) {
	var stat statistic
	fmt.Println("Progress (every 10 seconds):")

	for {
		err := json.Unmarshal(engine.GetProgress(), &stat)
		if err != nil {
			fmt.Println("Unmarshal error", err)
			break
		}

		select {
		case <-ctx.Done():
			return
		default:
			fmt.Println(
				runtime.NumGoroutine(),
				"folder",
				metrics(stat.Fetch),
				"fetch",
				metrics(stat.Size),
				"size",
				metrics(stat.Hash),
				"hash",
				metrics(stat.Match),
				"match",
				metrics(stat.Pack),
				"result",
				stat.Duration,
			)
		}
		time.Sleep(10 * time.Second)
	}
}

type progress struct {
	Count int64
	Size  int64
}

type direction struct {
	InpQueue progress
	OutQueue progress
}

type statistic struct {
	Start    time.Time
	Duration time.Duration
	Fetch    direction
	Size     direction
	Hash     direction
	Match    direction
	Pack     direction
}

func metrics(drc direction) string {
	ic := drc.InpQueue.Count
	oc := drc.OutQueue.Count
	return fmt.Sprintf("%d_%d_%d", ic, ic-oc, oc)
}

func printStatistic(engine fdd.SearchEngine) {
	var stat statistic

	err := json.Unmarshal(engine.GetProgress(), &stat)
	if err != nil {
		fmt.Println("Unmarshal error", err)
		return
	}
	fmt.Printf("------- TOTAL STATISTIC -------\n")
	fmt.Printf("Time of start:  %12v\n", stat.Start.Format("15:04:05"))
	fmt.Printf("Time of ended:  %12v\n", time.Now().Format("15:04:05"))
	fmt.Printf("Duration:       %12v\n", stat.Duration)
	fmt.Printf("Total processed <count (size Mb)>:\n")
	fmt.Printf("- folders:      %12d\n", stat.Fetch.InpQueue.Count)
	fmt.Printf("- files:        %12d (%.3f Mb)\n", stat.Size.InpQueue.Count, float64(stat.Size.InpQueue.Size)/1000000)
	fmt.Printf("Performance of filtration <inp-filtered-out (out %%)>:\n")
	fmt.Printf("- sizer:        %12d %12d %12d (%.2f %%)\n",
		stat.Size.InpQueue.Count,
		stat.Size.InpQueue.Count-stat.Hash.InpQueue.Count,
		stat.Hash.InpQueue.Count,
		float64(stat.Hash.InpQueue.Count)*100/float64(stat.Size.InpQueue.Count))
	fmt.Printf("- hasher:       %12d %12d %12d (%.2f %%)\n",
		stat.Hash.InpQueue.Count,
		stat.Hash.InpQueue.Count-stat.Match.InpQueue.Count,
		stat.Match.InpQueue.Count,
		float64(stat.Match.InpQueue.Count)*100/float64(stat.Hash.InpQueue.Count))
	fmt.Printf("- matcher:      %12d %12d %12d (%.2f %%)\n",
		stat.Match.InpQueue.Count,
		stat.Match.InpQueue.Count-stat.Pack.InpQueue.Count,
		stat.Pack.InpQueue.Count,
		float64(stat.Pack.InpQueue.Count)*100/float64(stat.Match.InpQueue.Count))
	fmt.Printf("Found duplicates <count (size Mb)>:\n")
	fmt.Printf("- groups of files:%10d\n", len(engine.GetResult().List))
	fmt.Printf("- files:          %10d (%.3f Mb)\n", stat.Pack.InpQueue.Count, float64(stat.Pack.InpQueue.Size)/1000000)
	fmt.Printf("File with result was created.\n")
	fmt.Printf("-------------------------------\n")
}
