package broker

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/terminus/internal/canon"
	"github.com/michaelquigley/terminus/internal/changeset"
	"github.com/michaelquigley/terminus/internal/coverage"
	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/michaelquigley/terminus/internal/report"
)

// AuditCoverageRequest selects one project rubric for a read-only full-tree
// territory audit.
type AuditCoverageRequest struct {
	RepoPath   string
	Rubric     string
	IncludeMap bool
}

// AuditCoverage evaluates one rubric against the repository's full tracked
// tree. it does not construct or call a reviewer, allocate a review id, create
// artifacts, or mutate broker job state.
func (b *Broker) AuditCoverage(ctx context.Context, req AuditCoverageRequest) (report.AuditResponse, error) {
	if err := ctx.Err(); err != nil {
		return report.AuditResponse{}, err
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		return report.AuditResponse{}, errs.New(errs.CodeUserError, "repo_path is required", nil, nil)
	}
	repoPath, err := filepath.Abs(req.RepoPath)
	if err != nil {
		return report.AuditResponse{}, errs.New(errs.CodeUserError, "resolve repo_path", err, nil)
	}
	store, err := canon.NewStore(b.options.CanonPath)
	if err != nil {
		return report.AuditResponse{}, errs.New(errs.CodeUserError, "open canon", err, nil)
	}
	rubric, project, rubricName, err := canon.LoadProjectRubric(store, repoPath, req.Rubric)
	if err != nil {
		return report.AuditResponse{}, errs.New(errs.CodeUserError, "load project rubric", err, map[string]any{"repo_path": repoPath, "rubric": rubricName})
	}
	composed, err := canon.Compose(store, rubric)
	if err != nil {
		return report.AuditResponse{}, errs.New(errs.CodeUserError, "compose rubric", err, nil)
	}
	cs, err := changeset.Full(ctx, repoPath)
	if err != nil {
		return report.AuditResponse{}, errs.New(errs.CodeUserError, "enumerate full tracked scope", err, map[string]any{"repo_path": repoPath})
	}
	assessment, err := coverage.Assess(ctx, project, composed, cs.Files, rubric.CoverageExclusions, coverage.Options{WithDeadPatterns: true, WithMap: req.IncludeMap})
	if err != nil {
		return report.AuditResponse{}, errs.New(errs.CodeUserError, "assess coverage", err, nil)
	}
	return *report.AuditResponseFrom(project, rubricName, assessment, req.IncludeMap), nil
}
