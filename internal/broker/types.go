package broker

import (
	"encoding/json"
	"time"

	"github.com/michaelquigley/terminus/internal/findings"
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
	Reviews []ReviewSummary `json:"reviews"`
}

type ReviewSummary struct {
	ReviewID      string `json:"review_id"`
	Project       string `json:"project"`
	Rubric        string `json:"rubric,omitempty"`
	State         string `json:"state"`
	ChangesetKind string `json:"changeset_kind"`
	StartedAt     string `json:"started_at"`
	CompletedAt   string `json:"completed_at,omitempty"`
	StatusPath    string `json:"status_path"`
	LogPath       string `json:"log_path,omitempty"`
}

type CollectReviewResponse struct {
	ReviewID     string                `json:"review_id"`
	Project      string                `json:"project"`
	Rubric       string                `json:"rubric,omitempty"`
	State        string                `json:"state"`
	Verdict      string                `json:"verdict"`
	Clean        bool                  `json:"clean"`
	Summary      string                `json:"summary"`
	LogPath      string                `json:"log_path"`
	PromptPath   string                `json:"prompt_path"`
	ReviewerName string                `json:"reviewer_name"`
	Raw          json.RawMessage       `json:"raw"`
	Findings     []TriageFindingOutput `json:"findings"`
	NextFinding  *TriageFindingOutput  `json:"next_finding,omitempty"`
	Guidance     string                `json:"guidance"`
}

type TriageFindingOutput struct {
	ID         string  `json:"id"`
	Quality    string  `json:"quality"`
	File       string  `json:"file"`
	Lines      string  `json:"lines"`
	Claim      string  `json:"claim"`
	Rationale  string  `json:"rationale"`
	Suggestion *string `json:"suggestion,omitempty"`
	Blocking   bool    `json:"blocking"`
}

type reviewResultFile struct {
	ReviewID     string                `json:"review_id"`
	Project      string                `json:"project"`
	Rubric       string                `json:"rubric,omitempty"`
	State        string                `json:"state"`
	Verdict      string                `json:"verdict"`
	Clean        bool                  `json:"clean"`
	Summary      string                `json:"summary"`
	LogPath      string                `json:"log_path"`
	PromptPath   string                `json:"prompt_path"`
	ReviewerName string                `json:"reviewer_name"`
	Raw          json.RawMessage       `json:"raw"`
	Findings     []TriageFindingOutput `json:"findings"`
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
