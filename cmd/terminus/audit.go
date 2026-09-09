package main

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/canon"
	"github.com/michaelquigley/terminus/internal/config"
	"github.com/michaelquigley/terminus/internal/report"
	"github.com/spf13/cobra"
)

func newAuditCoverageCommand(configPath *string, verbose *bool) *cobra.Command {
	var repoPath string
	var rubric string
	var includeMap bool
	cmd := &cobra.Command{
		Use:          "audit-coverage",
		Short:        "audit rubric territory coverage over the full tracked tree",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configureLogging(*verbose)
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			b := broker.New(broker.Options{CanonPath: cfg.CanonPath})
			result, err := b.AuditCoverage(cmd.Context(), broker.AuditCoverageRequest{RepoPath: repoPath, Rubric: rubric, IncludeMap: includeMap})
			if err != nil {
				return err
			}
			printAuditCoverage(cmd, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&repoPath, "repo", ".", "repo path to audit")
	cmd.Flags().StringVar(&rubric, "rubric", canon.DefaultRubric, "rubric name to audit")
	cmd.Flags().BoolVar(&includeMap, "include-map", false, "include per-file matching qualities and exclusions")
	return cmd
}

func printAuditCoverage(cmd *cobra.Command, result report.AuditResponse) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "project: %s\n", result.Project)
	fmt.Fprintf(out, "rubric: %s\n", result.Rubric)
	fmt.Fprintf(out, "scope: %s\n", result.FileScope)
	fmt.Fprintln(out, coverageSummary(result.Coverage))

	if result.Coverage != nil && len(result.Coverage.LocalQualities) > 0 && len(result.Coverage.UncoveredFiles) > 0 {
		fmt.Fprintln(out, "uncovered:")
		printPathGroups(out, result.Coverage.UncoveredFiles)
	}
	fmt.Fprintln(out, "dead patterns:")
	if len(result.DeadPatterns) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		for _, dead := range result.DeadPatterns {
			fmt.Fprintf(out, "  - %s: %s\n", dead.Quality.Ref, dead.Pattern)
		}
	}
	if result.CoverageMap != nil {
		fmt.Fprintln(out, "coverage map:")
		if len(*result.CoverageMap) == 0 {
			fmt.Fprintln(out, "  (empty)")
		}
		for _, group := range *result.CoverageMap {
			fmt.Fprintf(out, "  %s:\n", group.Directory)
			for _, file := range group.Files {
				fmt.Fprintf(out, "    - %s\n", file.File)
				fmt.Fprintf(out, "      qualities: %s\n", joinedQualityRefs(file.Qualities))
				fmt.Fprintf(out, "      exclusions: %s\n", joinedOrNone(file.ExclusionPatterns))
			}
		}
	}
}

func printPathGroups(out io.Writer, files []string) {
	groups := map[string][]string{}
	for _, file := range files {
		dir := path.Dir(file)
		groups[dir] = append(groups[dir], file)
	}
	dirs := make([]string, 0, len(groups))
	for dir := range groups {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		fmt.Fprintf(out, "  %s:\n", dir)
		for _, file := range groups[dir] {
			fmt.Fprintf(out, "    - %s\n", file)
		}
	}
}

func joinedQualityRefs(qualities []report.QualityRef) string {
	refs := make([]string, 0, len(qualities))
	for _, quality := range qualities {
		refs = append(refs, quality.Ref)
	}
	return joinedOrNone(refs)
}

func joinedOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}
