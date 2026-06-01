package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/httpx"
	"github.com/spf13/cobra"
)

func newWaveCmd(stdout, _ io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "wave", Short: "Inspect and control launch waves", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	cmd.AddCommand(newWaveStatusCmd(stdout))
	cmd.AddCommand(newWaveRunsCmd(stdout))
	cmd.AddCommand(newWaveCancelCmd(stdout))
	return cmd
}

func newWaveStatusCmd(out io.Writer) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{Use: "status <wave-id>", Short: "Show wave status", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		for {
			body, terminal, err := fetchWaveJSON(cmd.Context(), args[0], "")
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, string(body))
			if !follow || terminal {
				return nil
			}
			time.Sleep(time.Second)
		}
	}}
	cmd.Flags().BoolVar(&follow, "follow", false, "Poll until the wave reaches a terminal state")
	return cmd
}

func newWaveRunsCmd(out io.Writer) *cobra.Command {
	return &cobra.Command{Use: "runs <wave-id>", Short: "List runs in a wave", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		body, _, err := fetchWaveJSON(cmd.Context(), args[0], "runs")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(body))
		return nil
	}}
}

func newWaveCancelCmd(out io.Writer) *cobra.Command {
	return &cobra.Command{Use: "cancel <wave-id>", Short: "Cancel a wave", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		base, client, err := common.ResolveControlPlaneHTTP(cmd.Context())
		if err != nil {
			return err
		}
		endpoint := base.JoinPath("v1", "waves", strings.TrimSpace(args[0]), "cancel")
		req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, endpoint.String(), nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer httpx.DrainAndClose(resp)
		if resp.StatusCode != http.StatusOK {
			return httpx.WrapError("wave cancel", resp.Status, resp.Body)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxJSONBodyBytes))
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(body))
		return nil
	}}
}

func fetchWaveJSON(ctx context.Context, waveID, suffix string) ([]byte, bool, error) {
	base, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, false, err
	}
	parts := []string{"v1", "waves", strings.TrimSpace(waveID)}
	if strings.TrimSpace(suffix) != "" {
		parts = append(parts, strings.TrimSpace(suffix))
	}
	endpoint := base.JoinPath(parts...)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, false, httpx.WrapError("wave status", resp.Status, resp.Body)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxJSONBodyBytes))
	if err != nil {
		return nil, false, err
	}
	var envelope struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(body, &envelope)
	terminal := envelope.Status == "Finished" || envelope.Status == "Cancelled"
	return body, terminal, nil
}
