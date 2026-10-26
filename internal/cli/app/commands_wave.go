package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	sharedclient "github.com/iw2rmb/ploy/internal/client"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
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
		base, client, err := common.ResolveControlPlaneHTTP(cmd.Context())
		if err != nil {
			return err
		}
		get := sharedclient.GetWaveCommand{Client: client, BaseURL: base, WaveID: domaintypes.WaveID(strings.TrimSpace(args[0]))}
		if follow {
			_, err := (sharedclient.FollowWaveCommand{Get: get, OnUpdate: func(wave domainapi.WaveSummary) error {
				return writeWaveSummary(out, wave)
			}}).Run(cmd.Context())
			return err
		}
		wave, err := get.Run(cmd.Context())
		if err != nil {
			return err
		}
		return writeWaveSummary(out, wave)
	}}
	cmd.Flags().BoolVar(&follow, "follow", false, "Poll until the wave reaches a terminal state")
	return cmd
}

func newWaveRunsCmd(out io.Writer) *cobra.Command {
	return &cobra.Command{Use: "runs <wave-id>", Short: "List runs in a wave", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		body, err := fetchWaveRunsJSON(cmd.Context(), args[0])
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
		wave, err := (sharedclient.CancelWaveCommand{
			Client: client, BaseURL: base, WaveID: domaintypes.WaveID(strings.TrimSpace(args[0])),
		}).Run(cmd.Context())
		if err != nil {
			return err
		}
		return writeWaveSummary(out, wave)
	}}
}

func fetchWaveRunsJSON(ctx context.Context, waveID string) ([]byte, error) {
	base, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := base.JoinPath("v1", "waves", strings.TrimSpace(waveID), "runs")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("wave runs", resp.Status, resp.Body)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxJSONBodyBytes))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func writeWaveSummary(out io.Writer, wave domainapi.WaveSummary) error {
	body, err := json.Marshal(wave)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n\n", body)
	return err
}
