package main

import (
	"fmt"
	"path/filepath"

	"github.com/michaelquigley/terminus/internal/canon"
	"github.com/michaelquigley/terminus/internal/config"
	"github.com/spf13/cobra"
)

func newRubricsCommand(configPath *string, verbose *bool) *cobra.Command {
	var repoPath string

	cmd := &cobra.Command{
		Use:          "rubrics",
		Short:        "list the rubrics available for a project in the canon",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRubrics(cmd, *configPath, *verbose, repoPath)
		},
	}
	cmd.Flags().StringVar(&repoPath, "repo", ".", "repo path whose project rubrics to list")
	return cmd
}

func runRubrics(cmd *cobra.Command, configPath string, verbose bool, repoPath string) error {
	configureLogging(verbose)

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	store, err := canon.NewStore(cfg.CanonPath)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	project := canon.ProjectIdentity(abs)
	names, err := canon.ListRubrics(store, project)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "project: %s\n", project)
	if len(names) == 0 {
		fmt.Fprintln(out, "no rubrics found")
		return nil
	}
	for _, name := range names {
		suffix := ""
		if name == canon.DefaultRubric {
			suffix = " (default)"
		}
		fmt.Fprintf(out, "- %s%s\n", name, suffix)
	}
	return nil
}
