package transflow

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"encoding/json"
	"net/http"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// lightweight JSON unmarshal to avoid adding deps
func jsonUnmarshal(b []byte, v any) error {
	// use stdlib encoding/json
	return json.Unmarshal(b, v)
}

func printPlanSummary(b []byte) {
	var parsed struct {
		PlanID  string           `json:"plan_id"`
		Options []map[string]any `json:"options"`
	}
	if err := jsonUnmarshal(b, &parsed); err == nil && parsed.PlanID != "" && len(parsed.Options) > 0 {
		fmt.Printf("Planner produced plan (id=%s) with %d option(s):\n", parsed.PlanID, len(parsed.Options))
		for i, o := range parsed.Options {
			id, _ := o["id"].(string)
			typ, _ := o["type"].(string)
			fmt.Printf("  %d) %s (%s)\n", i+1, id, typ)
		}
	} else {
		fmt.Printf("Planner finished but plan.json validation failed or missing keys: %v\n", err)
	}
}

func printNextSummary(b []byte) {
	var parsed struct {
		Action string `json:"action"`
		Notes  string `json:"notes"`
	}
	if err := jsonUnmarshal(b, &parsed); err == nil && parsed.Action != "" {
		fmt.Printf("Reducer next action: %s", parsed.Action)
		if parsed.Notes != "" {
			fmt.Printf(" (%s)", parsed.Notes)
		}
		fmt.Println()
	} else {
		fmt.Printf("Reducer output invalid or missing keys: %v\n", err)
	}
}

// TransflowCmd provides the CLI entrypoint to run transflows
func TransflowCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Usage: ploy transflow run -f <transflow.yaml>")
		return
	}

	switch args[0] {
	case "run":
		if err := runTransflow(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Println("Usage: ploy transflow run -f <transflow.yaml>")
	}
}

// runTransflow handles the actual transflow execution
func runTransflow(args []string, controllerURL string) error {
	// Parse command line flags
	fs := flag.NewFlagSet("transflow run", flag.ContinueOnError)
	file := fs.String("f", "", "transflow YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	dryRun := fs.Bool("dry-run", false, "validate configuration without executing")
	testMode := fs.Bool("test-mode", false, "use mock implementations for testing (no real builds/pushes)")
	renderPlanner := fs.Bool("render-planner", false, "render planner inputs and HCL (no submission)")
	submitPlanner := fs.Bool("plan", false, "render and (optionally) submit planner job; prints paths. Set TRANSFLOW_SUBMIT=1 to submit.")
	submitReducer := fs.Bool("reduce", false, "render and (optionally) submit reducer job; prints next actions. Set TRANSFLOW_SUBMIT=1 to submit.")
	execFirst := fs.Bool("execute-first", false, "after reading plan.json, print which first option would be executed (sequential stub)")
	execLLM := fs.Bool("exec-llm-first", false, "render and optionally submit llm-exec job for the first plan option of type llm-exec")
	execORW := fs.Bool("exec-orw-first", false, "render and optionally submit orw-apply job for the first plan option of type orw-gen (requires recipe envs)")
	applyFirst := fs.Bool("apply-first", false, "after fetching diff (TRANSFLOW_DIFF_URL/TRANSFLOW_DIFF_PATH), clone repo, validate/apply diff, commit, and run build gate")
	verbose := fs.Bool("v", false, "verbose output")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("flag parsing failed: %w", err)
	}

	if *file == "" {
		return fmt.Errorf("missing -f <transflow.yaml>")
	}

	// Load and validate configuration
	config, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config from %s: %w", *file, err)
	}

	if *verbose {
		fmt.Printf("Loaded transflow config: %s\n", config.ID)
	}

	if *dryRun {
		fmt.Println("Configuration is valid")
		return nil
	}

	// Create working directory
	var workspaceDir string
	if *workDir != "" {
		workspaceDir = *workDir
		if err := os.MkdirAll(workspaceDir, 0755); err != nil {
			return fmt.Errorf("failed to create work directory %s: %w", workspaceDir, err)
		}
	} else {
		workspaceDir, err = os.MkdirTemp("", "transflow-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		defer os.RemoveAll(workspaceDir)
	}

	if *verbose {
		fmt.Printf("Using workspace: %s\n", workspaceDir)
	}

	// Create integrations and runner
	integrations := NewTransflowIntegrationsWithTestMode(controllerURL, workspaceDir, *testMode)
	runner, err := integrations.CreateConfiguredRunner(config)
	if err != nil {
		return fmt.Errorf("failed to create runner: %w", err)
	}

	if *renderPlanner {
		// Prepare minimal planner inputs/HCL without submitting
		assets, err := runner.RenderPlannerAssets()
		if err != nil {
			return fmt.Errorf("failed to render planner assets: %w", err)
		}
		fmt.Printf("Planner assets rendered:\n  inputs: %s\n  hcl:    %s\n", assets.InputsPath, assets.HCLPath)
		return nil
	}

	if *submitPlanner {
		assets, err := runner.RenderPlannerAssets()
		if err != nil {
			return fmt.Errorf("failed to render planner assets: %w", err)
		}
		// Substitute placeholders
		hclBytes, err := os.ReadFile(assets.HCLPath)
		if err != nil {
			return fmt.Errorf("failed to read planner HCL: %w", err)
		}
		model := os.Getenv("TRANSFLOW_MODEL")
		if model == "" {
			model = "gpt-4o-mini@2024-08-06"
		}
		toolsJSON := os.Getenv("TRANSFLOW_TOOLS")
		if toolsJSON == "" {
			toolsJSON = `{"file":{"allow":["src/**","pom.xml"]},"search":{"provider":"rg","allow":["src/**"]}}`
		}
		limitsJSON := os.Getenv("TRANSFLOW_LIMITS")
		if limitsJSON == "" {
			limitsJSON = `{"max_steps":8,"max_tool_calls":12,"timeout":"30m"}`
		}
		runID := fmt.Sprintf("%s-%d", runner.config.ID, time.Now().Unix())
		rendered := strings.NewReplacer(
			"${MODEL}", model,
			"${TOOLS_JSON}", toolsJSON,
			"${LIMITS_JSON}", limitsJSON,
			"${RUN_ID}", runID,
		).Replace(string(hclBytes))
		renderedPath := filepath.Join(filepath.Dir(assets.HCLPath), "planner.rendered.hcl")
		if err := os.WriteFile(renderedPath, []byte(rendered), 0644); err != nil {
			return fmt.Errorf("failed to write rendered HCL: %w", err)
		}
		fmt.Printf("Planner HCL rendered: %s\n", renderedPath)
		// Optionally submit if TRANSFLOW_SUBMIT=1
		if os.Getenv("TRANSFLOW_SUBMIT") == "1" {
			timeout := 30 * time.Minute
			if err := orchestration.SubmitAndWaitTerminal(renderedPath, timeout); err != nil {
				return fmt.Errorf("planner job failed: %w", err)
			}
			// Attempt to read plan.json from URL or locally (also support SeaweedFS filer via bucket/key)
			if url := os.Getenv("TRANSFLOW_PLAN_URL"); url != "" {
				resp, err := http.Get(url)
				if err == nil && resp.StatusCode == 200 {
					defer resp.Body.Close()
					b, _ := io.ReadAll(resp.Body)
					if err := validatePlanJSON(b); err != nil {
						fmt.Printf("plan.json schema invalid: %v\n", err)
					} else {
						printPlanSummary(b)
					}
				} else {
					if err != nil {
						fmt.Printf("Failed to fetch plan URL: %v\n", err)
					} else {
						fmt.Printf("Failed to fetch plan URL: %s\n", resp.Status)
					}
				}
			}
			if filer := os.Getenv("TRANSFLOW_FILER"); filer != "" {
				if bucket := os.Getenv("TRANSFLOW_BUCKET"); bucket != "" {
					if key := os.Getenv("TRANSFLOW_PLAN_KEY"); key != "" {
						url := strings.TrimRight(filer, "/") + "/" + strings.TrimLeft(bucket, "/") + "/" + strings.TrimLeft(key, "/")
						if resp, err := http.Get(url); err == nil && resp.StatusCode == 200 {
							defer resp.Body.Close()
							if b, err := io.ReadAll(resp.Body); err == nil {
								if err := validatePlanJSON(b); err != nil {
									fmt.Printf("plan.json schema invalid: %v\n", err)
								} else {
									printPlanSummary(b)
								}
							}
						}
					}
				}
			}
			// Attempt to read plan.json locally if provided
			planPath := os.Getenv("TRANSFLOW_PLAN_PATH")
			if planPath == "" {
				// Fallback to workspace/planner/out/plan.json (works if out volume maps locally)
				planPath = filepath.Join(filepath.Dir(renderedPath), "out", "plan.json")
			}
			var planBytes []byte
			if b, err := os.ReadFile(planPath); err == nil {
				planBytes = b
				if err := validatePlanJSON(planBytes); err != nil {
					fmt.Printf("plan.json schema invalid: %v\n", err)
				} else {
					printPlanSummary(planBytes)
				}
			} else {
				fmt.Println("Planner job completed. Could not read plan.json locally; set TRANSFLOW_PLAN_PATH or TRANSFLOW_PLAN_URL.")
			}
			// Optional reducer artifact print (if provided externally)
			if url := os.Getenv("TRANSFLOW_NEXT_URL"); url != "" {
				resp, err := http.Get(url)
				if err == nil && resp.StatusCode == 200 {
					defer resp.Body.Close()
					b, _ := io.ReadAll(resp.Body)
					if err := validateNextJSON(b); err != nil {
						fmt.Printf("next.json schema invalid: %v\n", err)
					} else {
						printNextSummary(b)
					}
				}
			}
			if np := os.Getenv("TRANSFLOW_NEXT_PATH"); np != "" {
				if b, err := os.ReadFile(np); err == nil {
					if err := validateNextJSON(b); err != nil {
						fmt.Printf("next.json schema invalid: %v\n", err)
					} else {
						printNextSummary(b)
					}
				}
			}
			if (*execFirst || *execLLM || *execORW || *applyFirst) && len(planBytes) > 0 {
				// Sequential stub: select first option and print intended action
				var parsed struct {
					PlanID  string           `json:"plan_id"`
					Options []map[string]any `json:"options"`
				}
				if err := jsonUnmarshal(planBytes, &parsed); err == nil && len(parsed.Options) > 0 {
					// Find first matching option for each request
					var first = parsed.Options[0]
					id, _ := first["id"].(string)
					typ, _ := first["type"].(string)
					if *execFirst {
						fmt.Printf("Sequential stub: would execute first option %s (%s) next.\n", id, typ)
						if typ == "llm-exec" {
							if path, err := runner.RenderLLMExecAssets(id); err == nil {
								fmt.Printf("Rendered llm_exec HCL: %s\n", path)
							}
						}
					}
					if *execLLM {
						// Find first llm-exec
						for _, o := range parsed.Options {
							if t, _ := o["type"].(string); t == "llm-exec" {
								lid, _ := o["id"].(string)
								if hcl, err := runner.RenderLLMExecAssets(lid); err == nil {
									fmt.Printf("Rendered llm_exec HCL: %s\n", hcl)
									// Substitute envs
									hb, _ := os.ReadFile(hcl)
									model := os.Getenv("TRANSFLOW_MODEL")
									if model == "" {
										model = "gpt-4o-mini@2024-08-06"
									}
									tools := os.Getenv("TRANSFLOW_TOOLS")
									if tools == "" {
										tools = `{"file":{"allow":["src/**","pom.xml"]}}`
									}
									limits := os.Getenv("TRANSFLOW_LIMITS")
									if limits == "" {
										limits = `{"max_steps":8,"max_tool_calls":12,"timeout":"30m"}`
									}
									runID := fmt.Sprintf("%s-%d", runner.config.ID, time.Now().Unix())
									rendered := strings.NewReplacer(
										"${MODEL}", model,
										"${TOOLS_JSON}", tools,
										"${LIMITS_JSON}", limits,
										"${RUN_ID}", runID,
									).Replace(string(hb))
									renderedPath := strings.ReplaceAll(hcl, ".rendered.hcl", ".rendered.submitted.hcl")
									_ = os.WriteFile(renderedPath, []byte(rendered), 0644)
									fmt.Printf("Rendered llm_exec HCL (substituted): %s\n", renderedPath)
									if os.Getenv("TRANSFLOW_SUBMIT") == "1" {
										if err := orchestration.SubmitAndWaitTerminal(renderedPath, 30*time.Minute); err != nil {
											fmt.Printf("llm-exec job failed: %v\n", err)
										} else {
											// Show where diff.patch would be
											diffPath := filepath.Join(filepath.Dir(renderedPath), "out", "diff.patch")
											fmt.Printf("llm-exec completed. diff.patch expected at: %s (or via TRANSFLOW_DIFF_URL/TRANSFLOW_DIFF_PATH).\n", diffPath)
										}
									} else {
										fmt.Println("Skipping llm-exec submission (unset TRANSFLOW_SUBMIT).")
									}
								}
								break
							}
						}
					}
					if *execORW {
						// Find first orw-gen
						for _, o := range parsed.Options {
							if t, _ := o["type"].(string); t == "orw-gen" {
								oid, _ := o["id"].(string)
								if hcl, err := runner.RenderORWApplyAssets(oid); err == nil {
									fmt.Printf("Rendered orw_apply HCL: %s\n", hcl)
									hb, _ := os.ReadFile(hcl)
									// Recipe envs required: TRANSFLOW_RECIPE_CLASS / TRANSFLOW_RECIPE_COORDS
									rclass := os.Getenv("TRANSFLOW_RECIPE_CLASS")
									if rclass == "" {
										rclass = "org.openrewrite.java.migrate.Java11toJava17"
									}
									rcoords := os.Getenv("TRANSFLOW_RECIPE_COORDS")
									rtimeout := os.Getenv("TRANSFLOW_RECIPE_TIMEOUT")
									if rtimeout == "" {
										rtimeout = "10m"
									}
									rendered := strings.NewReplacer(
										"${RECIPE_CLASS}", rclass,
										"${RECIPE_COORDS}", rcoords,
										"${RECIPE_TIMEOUT}", rtimeout,
									).Replace(string(hb))
									renderedPath := strings.ReplaceAll(hcl, ".rendered.hcl", ".rendered.submitted.hcl")
									_ = os.WriteFile(renderedPath, []byte(rendered), 0644)
									fmt.Printf("Rendered orw_apply HCL (substituted): %s\n", renderedPath)
									if os.Getenv("TRANSFLOW_SUBMIT") == "1" {
										if err := orchestration.SubmitAndWaitTerminal(renderedPath, 30*time.Minute); err != nil {
											fmt.Printf("orw-apply job failed: %v\n", err)
										} else {
											diffPath := filepath.Join(filepath.Dir(renderedPath), "out", "diff.patch")
											fmt.Printf("orw-apply completed. diff.patch expected at: %s\n", diffPath)
										}
									} else {
										fmt.Println("Skipping orw-apply submission (unset TRANSFLOW_SUBMIT).")
									}
								}
								break
							}
						}
					}
					if *applyFirst {
						// Fetch diff content path or URL
						var diffPath string
						if url := os.Getenv("TRANSFLOW_DIFF_URL"); url != "" {
							if resp, err := http.Get(url); err == nil && resp.StatusCode == 200 {
								b, _ := io.ReadAll(resp.Body)
								_ = resp.Body.Close()
								// Write to temp file in workspace
								dp := filepath.Join(runner.workspaceDir, "apply", "diff.patch")
								_ = os.MkdirAll(filepath.Dir(dp), 0755)
								_ = os.WriteFile(dp, b, 0644)
								diffPath = dp
							}
						}
						if diffPath == "" {
							if p := os.Getenv("TRANSFLOW_DIFF_PATH"); p != "" {
								diffPath = p
							}
						}
						if diffPath == "" {
							fmt.Println("Missing TRANSFLOW_DIFF_URL or TRANSFLOW_DIFF_PATH for --apply-first")
						} else {
							// Prepare repo and apply diff
							repoPath, _, err := runner.PrepareRepo(context.Background())
							if err != nil {
								fmt.Printf("PrepareRepo failed: %v\n", err)
							} else {
								if err := runner.ApplyDiffAndBuild(context.Background(), repoPath, diffPath); err != nil {
									fmt.Printf("Apply/build failed: %v\n", err)
								} else {
									fmt.Println("Apply/build succeeded")
								}
							}
						}
					}
				}
			}
		} else {
			fmt.Println("Skipping submission (unset TRANSFLOW_SUBMIT).")
		}
		return nil
	}

	if *submitReducer {
		assets, err := runner.RenderReducerAssets()
		if err != nil {
			return fmt.Errorf("failed to render reducer assets: %w", err)
		}
		// Substitute placeholders
		hclBytes, err := os.ReadFile(assets.HCLPath)
		if err != nil {
			return fmt.Errorf("failed to read reducer HCL: %w", err)
		}
		model := os.Getenv("TRANSFLOW_MODEL")
		if model == "" {
			model = "gpt-4o-mini@2024-08-06"
		}
		toolsJSON := os.Getenv("TRANSFLOW_TOOLS")
		if toolsJSON == "" {
			toolsJSON = `{"file":{"allow":["src/**","pom.xml"]}}`
		}
		limitsJSON := os.Getenv("TRANSFLOW_LIMITS")
		if limitsJSON == "" {
			limitsJSON = `{"max_steps":4,"max_tool_calls":8,"timeout":"15m"}`
		}
		runID := fmt.Sprintf("%s-%d", runner.config.ID, time.Now().Unix())
		rendered := strings.NewReplacer(
			"${MODEL}", model,
			"${TOOLS_JSON}", toolsJSON,
			"${LIMITS_JSON}", limitsJSON,
			"${RUN_ID}", runID,
		).Replace(string(hclBytes))
		renderedPath := filepath.Join(filepath.Dir(assets.HCLPath), "reducer.rendered.hcl")
		if err := os.WriteFile(renderedPath, []byte(rendered), 0644); err != nil {
			return fmt.Errorf("failed to write rendered HCL: %w", err)
		}
		fmt.Printf("Reducer HCL rendered: %s\n", renderedPath)
		if os.Getenv("TRANSFLOW_SUBMIT") == "1" {
			timeout := 15 * time.Minute
			if err := orchestration.SubmitAndWaitTerminal(renderedPath, timeout); err != nil {
				return fmt.Errorf("reducer job failed: %w", err)
			}
			// Fetch next.json via URL or local path
			if url := os.Getenv("TRANSFLOW_NEXT_URL"); url != "" {
				resp, err := http.Get(url)
				if err == nil && resp.StatusCode == 200 {
					defer resp.Body.Close()
					b, _ := io.ReadAll(resp.Body)
					printNextSummary(b)
				} else if err != nil {
					fmt.Printf("Failed to fetch next URL: %v\n", err)
				}
			}
			np := os.Getenv("TRANSFLOW_NEXT_PATH")
			if np == "" {
				np = filepath.Join(filepath.Dir(renderedPath), "out", "next.json")
			}
			if b, err := os.ReadFile(np); err == nil {
				printNextSummary(b)
			} else {
				fmt.Println("Reducer job completed. Could not read next.json; set TRANSFLOW_NEXT_PATH or TRANSFLOW_NEXT_URL.")
			}
		} else {
			fmt.Println("Skipping reducer submission (unset TRANSFLOW_SUBMIT).")
		}
		return nil
	}

	// Execute the transflow
	ctx := context.Background()
	startTime := time.Now()

	fmt.Printf("Starting transflow execution: %s\n", config.ID)

	result, err := runner.Run(ctx)
	if err != nil {
		fmt.Printf("Transflow failed after %v\n", time.Since(startTime))
		if result != nil {
			fmt.Println(result.Summary())
		}
		return fmt.Errorf("transflow execution failed: %w", err)
	}

	// Print results
	fmt.Printf("Transflow completed successfully in %v\n", time.Since(startTime))
	if *verbose && result != nil {
		fmt.Println(result.Summary())
	} else {
		fmt.Printf("Branch: %s\n", result.BranchName)
		if result.BuildVersion != "" {
			fmt.Printf("Build: %s\n", result.BuildVersion)
		}
	}

	return nil
}
