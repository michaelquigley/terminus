package monitor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/michaelquigley/terminus/internal/report"
)

const (
	StatusFileName = "status.json"
	StateRunning   = "running"
	StateCompleted = "completed"
	StateFailed    = "failed"
)

type ReviewerInfo struct {
	Name  string
	Impl  string
	Model string `dd:",+omitempty"`
}

type ReviewStatus struct {
	ReviewID      string
	Project       string
	Rubric        string `dd:",+omitempty"`
	State         string
	ChangesetKind string
	Reviewer      ReviewerInfo
	StartedAt     string
	UpdatedAt     string
	CompletedAt   string `dd:",+omitempty"`
	StatusPath    string
	LogPath       string `dd:",+omitempty"`
	Error         *errs.Info
	Files         []string      `dd:",+omitempty"`
	Qualities     []QualityInfo `dd:",+omitempty"`
	// ExcludedQualities records the rubric qualities territory narrowing
	// dropped from this review, so the status of a running review shows what
	// it covers before the verdict lands.
	ExcludedQualities []QualityInfo `dd:",+omitempty"`
	// Coverage is the territory-coverage assessment computed before the
	// reviewer starts, carried unchanged from the first status write through
	// completion or failure. nil on reviews recorded before coverage
	// reporting existed: absence is unavailable, never an assessed empty
	// gap list, and must not be filled in from the current canon.
	Coverage *report.Coverage
}

type QualityInfo struct {
	ID       string
	Ref      string
	Blocking bool
}

func ReviewDir(logDestination string, project string, reviewID string) string {
	return filepath.Join(logDestination, project, reviewID)
}

func StatusPath(reviewDir string) string {
	return filepath.Join(reviewDir, StatusFileName)
}

func WriteStatus(path string, status ReviewStatus) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := dd.UnbindJSON(status)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".status-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func ReadStatus(path string) (ReviewStatus, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ReviewStatus{}, err
	}
	var status ReviewStatus
	if err := dd.BindJSON(&status, raw); err != nil {
		return ReviewStatus{}, err
	}
	return status, nil
}

func FindStatusPath(logDestination string, project string, reviewID string) (string, error) {
	if project != "" {
		path := StatusPath(ReviewDir(logDestination, project, reviewID))
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		return path, nil
	}
	projects, err := os.ReadDir(logDestination)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		path := StatusPath(ReviewDir(logDestination, p.Name(), reviewID))
		if _, err := os.Stat(path); err == nil {
			matches = append(matches, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("review %q not found under %s", reviewID, logDestination)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("review %q matched multiple projects; pass --project", reviewID)
	}
}

func NowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
