package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/canon"
	"github.com/michaelquigley/terminus/internal/changeset"
	"github.com/michaelquigley/terminus/internal/config"
	"github.com/michaelquigley/terminus/internal/wiring"
	"github.com/spf13/cobra"
)

func newReviewCommand(configPath *string, verbose *bool) *cobra.Command {
	var repoPath string
	var kind string
	var rubric string
	var qualities []string
	var blocking bool

	cmd := &cobra.Command{
		Use:          "review [paths...]",
		Short:        "run a Terminus review in the foreground",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := reviewRequest(repoPath, kind, args)
			if err != nil {
				return err
			}
			if len(qualities) > 0 && cmd.Flags().Changed("rubric") {
				return errors.New("cannot combine --rubric and --quality; --quality runs an ad-hoc review")
			}
			if cmd.Flags().Changed("blocking") && len(qualities) == 0 {
				return errors.New("--blocking only applies with --quality")
			}
			req.Rubric = rubric
			req.Qualities = append([]string(nil), qualities...)
			req.QualitiesBlocking = blocking
			// when --kind was left at its default and the working tree is clean,
			// a working-tree review would select nothing and report a vacuous
			// clean verdict. promote to a full review so a bare `terminus review`
			// on a committed repo reviews the project instead of nothing. an
			// explicit --kind is always honored.
			if !cmd.Flags().Changed("kind") && req.ChangesetKind == changeset.KindWorkingTree {
				empty, err := workingTreeEmpty(cmd.Context(), req.RepoPath)
				if err != nil {
					return err
				}
				if empty {
					req.ChangesetKind = changeset.KindFull
					fmt.Fprintln(cmd.OutOrStdout(), "working tree clean; reviewing full tracked repo (--kind full)")
				}
			}
			return runReview(cmd, *configPath, *verbose, req)
		},
	}
	cmd.Flags().StringVar(&repoPath, "repo", ".", "repo path to review")
	cmd.Flags().StringVar(&kind, "kind", changeset.KindWorkingTree, "changeset kind: working-tree, paths, or full")
	cmd.Flags().StringVar(&rubric, "rubric", canon.DefaultRubric, "rubric name to select qualities from the canon")
	cmd.Flags().StringArrayVar(&qualities, "quality", nil, "canon quality ref to review against directly (repeatable); runs an ad-hoc review, bypassing the rubric")
	cmd.Flags().BoolVar(&blocking, "blocking", false, "treat --quality entries as blocking (ad-hoc reviews only)")
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

// workingTreeEmpty reports whether the repo has no uncommitted changes. it
// reuses changeset.WorkingTree so the CLI's notion of "clean" stays byte-for-byte
// the dirty set the broker would otherwise review, rather than a parallel check
// that could drift from it.
func workingTreeEmpty(ctx context.Context, repoPath string) (bool, error) {
	cs, err := changeset.WorkingTree(ctx, repoPath)
	if err != nil {
		return false, err
	}
	return len(cs.Files) == 0, nil
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
	b, err := wiring.NewBroker(cfg)
	if err != nil {
		return err
	}
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
	if result.Rubric != "" {
		fmt.Fprintf(out, "rubric: %s\n", result.Rubric)
	}
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
