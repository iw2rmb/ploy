package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/iw2rmb/ploy/internal/testutil"
)

func main() {
	var (
		testType       = flag.String("test-type", "chttp", "Type of test being monitored (chttp, legacy)")
		duration       = flag.Int("duration", 60, "Monitoring duration in seconds")
		interval       = flag.Int("interval", 5, "Sample interval in seconds")
		targetMemory   = flag.Int("target-memory", 100, "Target memory usage in MB")
		processPattern = flag.String("process-pattern", "pylint-chttp", "Process pattern to monitor")
		outputFile     = flag.String("output", "", "Output file path (default: stdout)")
		help           = flag.Bool("help", false, "Show help message")
	)

	flag.Parse()

	if *help {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nMonitors resource usage during performance testing.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -test-type=chttp -duration=120 -output=results.json\n", os.Args[0])
		os.Exit(0)
	}

	// Create resource monitor
	monitor := testutil.NewResourceMonitor(*testType, *targetMemory, *interval)
	monitor.SetProcessPattern(*processPattern)

	fmt.Printf("Starting resource monitoring for %s (%ds duration, %ds interval)\n", 
		*testType, *duration, *interval)
	fmt.Printf("Target memory usage: %d MB\n", *targetMemory)
	fmt.Printf("Process pattern: %s\n", *processPattern)

	// Start monitoring
	result, err := monitor.MonitorForDuration(time.Duration(*duration) * time.Second)
	if err != nil {
		log.Fatalf("Monitoring failed: %v", err)
	}

	// Print summary to stderr
	fmt.Fprintf(os.Stderr, "\nMonitoring complete!\n")
	fmt.Fprintf(os.Stderr, "Samples collected: %d\n", len(result.Samples))
	fmt.Fprintf(os.Stderr, "Max memory usage: %d MB (target: %d MB)\n", 
		result.Summary.MaxMemoryMB, result.TargetMemoryMB)
	fmt.Fprintf(os.Stderr, "Avg memory usage: %.1f MB\n", result.Summary.AvgMemoryMB)
	fmt.Fprintf(os.Stderr, "Max load average: %.2f\n", result.Summary.MaxLoad)
	fmt.Fprintf(os.Stderr, "Avg load average: %.2f\n", result.Summary.AvgLoad)
	fmt.Fprintf(os.Stderr, "Meets memory target: %t\n", result.Summary.MeetsMemoryTarget)

	// Output result
	if *outputFile != "" {
		if err := result.SaveToFile(*outputFile); err != nil {
			log.Fatalf("Failed to save result to file: %v", err)
		}
		fmt.Fprintf(os.Stderr, "Results saved to: %s\n", *outputFile)
	} else {
		// Output JSON to stdout
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Fatalf("Failed to marshal result: %v", err)
		}
		fmt.Printf("%s\n", data)
	}
}