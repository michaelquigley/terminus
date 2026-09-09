package broker

import (
	"encoding/json"
	"time"

	"github.com/michaelquigley/terminus/internal/findings"
	"github.com/michaelquigley/terminus/internal/report"
	"github.com/michaelquigley/theharnessbody/reviewer"
)

type Options struct {
	LogDestination string
	ConfigPath     string
	CanonPath      string
	Reviewer       reviewer.Reviewer
	ReviewerInfo   ReviewerInfo
}

type ReviewerInfo struct {
	Name  string
	Impl  string
	Model string
}

type StartReviewRequest struct {
	RepoPath      string
	ChangesetKind string
	Paths         []string
	Rubric        string
	// Qualities, when non-empty, runs an ad-hoc review against these canon
	// quality refs directly, bypassing rubric resolution (Rubric is ignored).
	Qualities         []string
	QualitiesBlocking bool
}

type StartReviewResponse struct {
	ReviewID       string
	Project        string
	State          string
	Reviewer       string
	StartedAt      time.Time
	StatusPath     string
	MonitorCommand string
	NextAction     string
}

type CollectReviewRequest struct {
	Project  string
	ReviewID string
}

type ListReviewsResponse struct {
	Reviews []ReviewSummary
}

type ReviewSummary struct {
	ReviewID      string
	Project       string
	Rubric        string `dd:",+omitempty"`
	State         string
	ChangesetKind string
	StartedAt     string
	CompletedAt   string `dd:",+omitempty"`
	StatusPath    string
	LogPath       string `dd:",+omitempty"`
}

type CollectReviewResponse struct {
	ReviewID          string
	Project           string
	Rubric            string `dd:",+omitempty"`
	QualitiesSelected int
	ExcludedQualities []ExcludedQuality `dd:",+omitempty"`
	State             string
	Verdict           string
	Clean             bool
	Summary           string
	LogPath           string
	PromptPath        string
	ReviewerName      string
	Raw               json.RawMessage
	Findings          []TriageFindingOutput
	NextFinding       *TriageFindingOutput
	Guidance          string
	// Coverage is the territory-coverage assessment computed before the
	// reviewer starts, carried unchanged through status, result, and collect.
	// new reviews always set it: rubric reviews carry an assessed object,
	// ad-hoc reviews carry assessed:false with reason ad_hoc. nil only on
	// historical results recorded before coverage reporting existed, which
	// means coverage is unavailable for that review.
	Coverage *report.Coverage
}

// ExcludedQuality records a rubric-listed quality that territory narrowing
// dropped from a review, so a verdict stays honest about how much of the
// rubric it actually covered.
type ExcludedQuality struct {
	ID       string
	Ref      string
	Blocking bool
}

type TriageFindingOutput struct {
	ID         string
	Quality    string
	File       string
	Lines      string
	Claim      string
	Rationale  string
	Suggestion *string
	Blocking   bool
}

type reviewResultFile struct {
	ReviewID          string
	Project           string
	Rubric            string `dd:",+omitempty"`
	QualitiesSelected int
	ExcludedQualities []ExcludedQuality `dd:",+omitempty"`
	State             string
	Verdict           string
	Clean             bool
	Summary           string
	LogPath           string
	PromptPath        string
	ReviewerName      string
	Raw               json.RawMessage
	Findings          []TriageFindingOutput
	Coverage          *report.Coverage
}

func classifiedToTriage(classified []findings.Classified) []TriageFindingOutput {
	out := make([]TriageFindingOutput, 0, len(classified))
	for _, c := range classified {
		f := c.Finding
		out = append(out, TriageFindingOutput{
			ID:         f.ID,
			Quality:    f.Quality,
			File:       f.File,
			Lines:      f.Lines,
			Claim:      f.Claim,
			Rationale:  f.Rationale,
			Suggestion: f.Suggestion,
			Blocking:   c.Blocking,
		})
	}
	return out
}
