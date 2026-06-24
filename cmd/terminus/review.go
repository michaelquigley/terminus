package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/changeset"
	"github.com/michaelquigley/terminus/internal/config"
	"github.com/michaelquigley/terminus/internal/wiring"
	"github.com/spf13/cobra"
)

func newReviewCommand(configPath *string, verbose *bool) *cobra.Command {
	var repoPath string
	var kind string

	cmd := &cobra.Command{
		Use:          "review [paths...]",
		Short:        "run a Terminus review in the foreground",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := reviewRequest(repoPath, kind, args)
			if err != nil {
				return err
			}
			return runReview(cmd, *configPath, *verbose, req)
		},
	}
	cmd.Flags().StringVar(&repoPath, "repo", ".", "repo path to review")
	cmd.Flags().StringVar(&kind, "kind", changeset.KindWorkingTree, "changeset kind: working-tree, paths, or full")
	return cmd
}

func reviewRequest(repoPath string, kind string, args []string) (broker.StartReviewRequest, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = changeset.KindWorkingTree
	}
	switch kind {
	case changeset.KindWorkingTree:
		if len(args) > 0 {
			return broker.StartReviewRequest{}, errors.New("path arguments require --kind paths")
		}
	case changeset.KindPaths:
		if len(args) == 0 {
			return broker.StartReviewRequest{}, errors.New("--kind paths requires at least one path argument")
		}
	case changeset.KindFull:
		if len(args) > 0 {
			return broker.StartReviewRequest{}, errors.New("--kind full does not accept path arguments")
		}
	default:
		return broker.StartReviewRequest{}, fmt.Errorf("unknown changeset kind %q", kind)
	}
	return broker.StartReviewRequest{
		RepoPath:      repoPath,
		ChangesetKind: kind,
		Paths:         append([]string(nil), args...),
	}, nil
}

func runReview(cmd *cobra.Command, configPath string, verbose bool, req broker.StartReviewRequest) error {
	configureLogging(verbose)

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := cfg.EnsureLogDestination(); err != nil {
		return err
	}
	options, err := wiring.BrokerOptions(cfg)
	if err != nil {
		return err
	}
	b := broker.New(options)
	result, err := b.RunReview(cmd.Context(), req)
	if err != nil {
		return err
	}
	printReviewResult(cmd, result)
	return nil
}

func printReviewResult(cmd *cobra.Command, result broker.CollectReviewResponse) {
	out := cmd.OutOrStdout()
	blocking, advisory := findingCounts(result.Findings)

	fmt.Fprintf(out, "review '%s' completed\n", result.ReviewID)
	fmt.Fprintf(out, "project: %s\n", result.Project)
	fmt.Fprintf(out, "verdict: %s\n", result.Verdict)
	fmt.Fprintf(out, "clean: %t\n", result.Clean)
	if result.ReviewerName != "" {
		fmt.Fprintf(out, "reviewer: %s\n", result.ReviewerName)
	}
	fmt.Fprintf(out, "findings: %d blocking, %d advisory\n", blocking, advisory)
	if result.PromptPath != "" {
		fmt.Fprintf(out, "prompt: %s\n", result.PromptPath)
	}
	if result.LogPath != "" {
		fmt.Fprintf(out, "log: %s\n", result.LogPath)
	}
	if len(result.Findings) == 0 {
		return
	}
	fmt.Fprintln(out)
	for _, finding := range result.Findings {
		kind := "advisory"
		if finding.Blocking {
			kind = "blocking"
		}
		fmt.Fprintf(out, "- [%s] %s (%s) %s:%s\n", kind, finding.ID, finding.Quality, finding.File, finding.Lines)
		fmt.Fprintf(out, "  %s\n", finding.Claim)
		if finding.Suggestion != nil && strings.TrimSpace(*finding.Suggestion) != "" {
			fmt.Fprintf(out, "  suggestion: %s\n", strings.TrimSpace(*finding.Suggestion))
		}
	}
}

func findingCounts(findings []broker.TriageFindingOutput) (blocking int, advisory int) {
	for _, finding := range findings {
		if finding.Blocking {
			blocking++
		} else {
			advisory++
		}
	}
	return blocking, advisory
}
