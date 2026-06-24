package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/michaelquigley/push/build"
	"github.com/michaelquigley/terminus/internal/config"
	"github.com/michaelquigley/terminus/internal/mcpserver"
	"github.com/michaelquigley/terminus/internal/monitor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func main() {
	configureLogging(false)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := newRootCommand()
	if err := root.ExecuteContext(ctx); err != nil {
		dl.Fatalf("terminus failed: %v", err)
	}
}

func newRootCommand() *cobra.Command {
	var configPath string
	var verbose bool

	root := &cobra.Command{
		Use:           "terminus",
		Short:         "run Terminus review and MCP commands",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "optional path to terminus.yaml")
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose stderr logging")
	root.AddCommand(newServeCommand(&configPath, &verbose))
	root.AddCommand(newReviewCommand(&configPath, &verbose))
	root.AddCommand(newRubricsCommand(&configPath, &verbose))
	root.AddCommand(newMonitorCommand(&configPath))
	root.AddCommand(build.NewVersionCmd("terminus"))
	return root
}

func newServeCommand(configPath *string, verbose *bool) *cobra.Command {
	return &cobra.Command{
		Use:          "serve",
		Short:        "run the Terminus MCP server over stdio",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(cmd, *configPath, *verbose)
		},
	}
}

func runServer(cmd *cobra.Command, configPath string, verbose bool) error {
	configureLogging(verbose)

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := cfg.EnsureLogDestination(); err != nil {
		return err
	}
	server, _, err := mcpserver.New(cfg)
	if err != nil {
		return err
	}
	err = server.Run(cmd.Context(), &mcp.StdioTransport{})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func newMonitorCommand(configPath *string) *cobra.Command {
	var project string
	var wait bool

	cmd := &cobra.Command{
		Use:          "monitor <review-id>",
		Short:        "monitor a Terminus review",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			return monitorReview(cmd, cfg.LogDestination, project, args[0], wait)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project name; when omitted Terminus searches project directories")
	cmd.Flags().BoolVar(&wait, "wait", false, "poll status.json until the review completes or fails")
	return cmd
}

func monitorReview(cmd *cobra.Command, logDestination string, project string, reviewID string, wait bool) error {
	statusPath, err := monitor.FindStatusPath(logDestination, project, reviewID)
	if err != nil {
		return err
	}
	status, err := monitor.ReadStatus(statusPath)
	if err != nil {
		return err
	}
	printReviewStatus(cmd, status)
	if !wait || status.State != monitor.StateRunning {
		return nil
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-ticker.C:
			status, err := monitor.ReadStatus(statusPath)
			if err != nil {
				return err
			}
			if status.State == monitor.StateRunning {
				continue
			}
			fmt.Fprintln(cmd.OutOrStdout())
			printReviewStatus(cmd, status)
			return nil
		}
	}
}

func printReviewStatus(cmd *cobra.Command, status monitor.ReviewStatus) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "review '%s' %s\n", status.ReviewID, status.State)
	fmt.Fprintf(out, "project: %s\n", status.Project)
	if status.Rubric != "" {
		fmt.Fprintf(out, "rubric: %s\n", status.Rubric)
	}
	fmt.Fprintf(out, "changeset: %s\n", status.ChangesetKind)
	if status.Reviewer.Name != "" {
		fmt.Fprintf(out, "reviewer: %s\n", status.Reviewer.Name)
	}
	if status.StartedAt != "" {
		fmt.Fprintf(out, "started: %s\n", status.StartedAt)
	}
	if status.CompletedAt != "" {
		fmt.Fprintf(out, "completed: %s\n", status.CompletedAt)
	}
	if status.LogPath != "" {
		fmt.Fprintf(out, "log: %s\n", status.LogPath)
	}
	if status.Error != nil {
		fmt.Fprintf(out, "error: %s - %s\n", status.Error.Code, status.Error.Message)
	}
}

func configureLogging(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	dl.Init(dl.DefaultOptions().
		SetOutput(os.Stderr).
		SetTrimPrefix("github.com/michaelquigley/terminus/").
		SetLevel(level))
}
