package mods

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"
)

func watchMod(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod watch", flag.ContinueOnError)
	id := fs.String("id", "", "execution id to watch")
	interval := fs.Duration("interval", 2*time.Second, "poll interval")
	noSSE := fs.Bool("no-sse", false, "disable SSE and use polling")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("flag parsing failed: %w", err)
	}
	if *id == "" {
		return fmt.Errorf("missing -id <mod_id>")
	}
	base := controllerURL
	if base == "" {
		base = GetDefaultControllerURL()
	}
	base = strings.TrimRight(base, "/")
	if !strings.HasSuffix(base, "/v1") {
		base = base + "/v1"
	}
	statusURL := base + "/mods/" + *id + "/status"
	artsURL := base + "/mods/" + *id + "/artifacts"

	if !*noSSE {
		if err := watchModSSE(base, *id); err == nil {
			return nil
		}
		fmt.Println("SSE unavailable; falling back to polling...")
	}

	seen := 0
	lastStatus := ""
	client := &http.Client{Timeout: 10 * time.Second}
	fmt.Printf("Watching mod %s (poll %s)\n", *id, interval.String())
	for {
		req, _ := http.NewRequest(http.MethodGet, statusURL, nil)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("status error: %v\n", err)
			time.Sleep(*interval)
			continue
		}
		var st struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Phase    string `json:"phase"`
			Duration string `json:"duration"`
			Overdue  bool   `json:"overdue"`
			Steps    []struct {
				Step    string    `json:"step"`
				Phase   string    `json:"phase"`
				Level   string    `json:"level"`
				Message string    `json:"message"`
				Time    time.Time `json:"time"`
			} `json:"steps"`
			Result map[string]interface{} `json:"result"`
			Error  string                 `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&st)
		_ = resp.Body.Close()

		if st.Status != lastStatus || st.Phase != "" {
			fmt.Printf("[%s] phase=%s status=%s duration=%s overdue=%v\n", time.Now().Format(time.RFC3339), st.Phase, st.Status, st.Duration, st.Overdue)
			lastStatus = st.Status
		}

		if len(st.Steps) > seen {
			for _, ev := range st.Steps[seen:] {
				ts := ev.Time.Format(time.RFC3339)
				lvl := ev.Level
				if lvl == "" {
					lvl = "info"
				}
				step := ev.Step
				if step == "" {
					step = "?"
				}
				fmt.Printf("%s [%s] %s: %s\n", ts, lvl, step, ev.Message)
			}
			seen = len(st.Steps)
		}

		if st.Status == "completed" || st.Status == "failed" || st.Status == "cancelled" {
			if st.Error != "" {
				fmt.Printf("Error: %s\n", st.Error)
			}
			req2, _ := http.NewRequest(http.MethodGet, artsURL, nil)
			resp2, err2 := client.Do(req2)
			if err2 == nil && resp2.StatusCode == 200 {
				var arts struct {
					Artifacts map[string]string `json:"artifacts"`
				}
				_ = json.NewDecoder(resp2.Body).Decode(&arts)
				_ = resp2.Body.Close()
				if key := arts.Artifacts["error_log"]; key != "" {
					dl := base + "/mods/" + *id + "/artifacts/error_log"
					if r3, e3 := client.Get(dl); e3 == nil && r3.StatusCode == 200 {
						defer func() { _ = r3.Body.Close() }()
						fmt.Println("--- error.log ---")
						_, _ = io.Copy(os.Stdout, r3.Body)
						fmt.Println("\n--- end error.log ---")
					}
				}
			}
			break
		}
		time.Sleep(*interval)
	}
	return nil
}

func watchModSSE(base, id string) error {
	url := fmt.Sprintf("%s/mods/%s/logs?follow=true", base, id)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected SSE response: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if ctype := resp.Header.Get("Content-Type"); ctype != "" {
		if mt, _, err := mime.ParseMediaType(ctype); err == nil {
			if mt != "text/event-stream" {
				return fmt.Errorf("unexpected SSE response: %d %s", resp.StatusCode, ctype)
			}
		} else if !strings.HasPrefix(strings.ToLower(ctype), "text/event-stream") {
			return fmt.Errorf("unexpected SSE response: %d %s", resp.StatusCode, ctype)
		}
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	curEvent := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			curEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			switch curEvent {
			case "init":
				fmt.Printf("[SSE] %s\n", data)
			case "meta":
				fmt.Printf("[meta] %s\n", data)
			case "step":
				var ev struct{ Level, Step, Message string }
				if _ = json.Unmarshal([]byte(data), &ev); ev.Step != "" {
					lvl := ev.Level
					if lvl == "" {
						lvl = "info"
					}
					fmt.Printf("[%s] %s: %s\n", lvl, ev.Step, ev.Message)
				} else {
					fmt.Printf("%s\n", data)
				}
			case "log":
				fmt.Printf("[log]\n%s\n", data)
			case "ping":
				// ignore
			case "end":
				fmt.Printf("[end] %s\n", data)
				return nil
			default:
				fmt.Printf("[%s] %s\n", curEvent, data)
			}
		}
		if line == "" {
			curEvent = ""
		}
	}
	return scanner.Err()
}
