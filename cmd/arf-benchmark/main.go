package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"
	
	"github.com/iw2rmb/ploy/api/arf"
	"gopkg.in/yaml.v3"
)

// Simple test runner for the ARF benchmark suite
func main() {
	var configFile string
	var outputDir string
	var verbose bool
	
	flag.StringVar(&configFile, "config", "controller/arf/benchmark_configs/minimal_test.yaml", "Benchmark configuration file")
	flag.StringVar(&outputDir, "output", "./benchmark_results", "Output directory for results")
	flag.BoolVar(&verbose, "verbose", true, "Enable verbose output")
	flag.Parse()
	
	fmt.Println("ARF Benchmark Test Runner")
	fmt.Println("=========================")
	fmt.Printf("Config: %s\n", configFile)
	fmt.Printf("Output: %s\n", outputDir)
	fmt.Println()
	
	// Load configuration
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	
	var config arf.BenchmarkConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}
	
	// Override output directory if specified
	if outputDir != "" {
		config.OutputDir = outputDir
	}
	
	// Create benchmark suite
	fmt.Println("Creating benchmark suite...")
	
	// Create simple logger for standalone command
	logger := func(level, stage, message, details string) {
		timestamp := time.Now().Format("15:04:05")
		if details != "" {
			fmt.Printf("[%s] [%s] [%s] %s - %s\n", timestamp, level, stage, message, details)
		} else {
			fmt.Printf("[%s] [%s] [%s] %s\n", timestamp, level, stage, message)
		}
	}
	
	suite, err := arf.NewBenchmarkSuite(&config, logger)
	if err != nil {
		log.Fatalf("Failed to create benchmark suite: %v", err)
	}
	
	// Run benchmark
	fmt.Println("Starting benchmark test...")
	fmt.Printf("Repository: %s\n", config.RepoURL)
	fmt.Printf("Max iterations: %d\n", config.MaxIterations)
	fmt.Printf("LLM provider: %s (%s)\n", config.LLMProvider, config.LLMModel)
	fmt.Println()
	
	ctx := context.Background()
	
	result, err := suite.Run(ctx)
	if err != nil {
		log.Printf("Warning: Benchmark completed with errors: %v", err)
	}
	
	// Display results
	fmt.Println("\n=== Benchmark Results ===")
	fmt.Printf("Total duration: %s\n", result.TotalDuration)
	fmt.Printf("Iterations run: %d\n", result.Summary.TotalIterations)
	fmt.Printf("Successful: %d\n", result.Summary.SuccessfulIterations)
	fmt.Printf("Failed: %d\n", result.Summary.FailedIterations)
	
	if result.Summary.TotalIterations > 0 {
		successRate := float64(result.Summary.SuccessfulIterations) / float64(result.Summary.TotalIterations) * 100
		fmt.Printf("Success rate: %.1f%%\n", successRate)
	}
	
	fmt.Printf("Files modified: %d\n", result.Summary.TotalFilesModified)
	fmt.Printf("Lines changed: %d\n", result.Summary.TotalLinesChanged)
	
	// Display iteration details
	if verbose && len(result.Iterations) > 0 {
		fmt.Println("\n=== Iteration Details ===")
		for i, iter := range result.Iterations {
			fmt.Printf("\nIteration %d:\n", i+1)
			fmt.Printf("  Status: %s\n", iter.Status)
			fmt.Printf("  Duration: %s\n", iter.Duration)
			fmt.Printf("  Stages: %d\n", len(iter.Stages))
			
			for _, stage := range iter.Stages {
				fmt.Printf("    - %s: %s (%s)\n", stage.Name, stage.Status, stage.Duration)
			}
			
			if len(iter.Errors) > 0 {
				fmt.Printf("  Errors: %d\n", len(iter.Errors))
				for _, err := range iter.Errors {
					fmt.Printf("    - %s: %s\n", err.Type, err.Message)
				}
			}
			
			if len(iter.Diffs) > 0 {
				fmt.Printf("  Changes: %d files\n", len(iter.Diffs))
			}
		}
	}
	
	// Save results to file
	resultsFile := fmt.Sprintf("%s/benchmark_%s_%d.json", 
		config.OutputDir, config.Name, time.Now().Unix())
	
	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal results: %v", err)
	} else {
		if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
			log.Printf("Failed to create output directory: %v", err)
		} else if err := ioutil.WriteFile(resultsFile, resultData, 0644); err != nil {
			log.Printf("Failed to save results: %v", err)
		} else {
			fmt.Printf("\nResults saved to: %s\n", resultsFile)
		}
	}
	
	// Exit with appropriate code
	if result.Summary.FailedIterations > 0 {
		os.Exit(1)
	}
	os.Exit(0)
}