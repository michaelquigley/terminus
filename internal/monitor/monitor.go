package monitor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/terminus/internal/errs"
)

const (
	StatusFileName = "status.json"
	StateRunning   = "running"
	StateCompleted = "completed"
	StateFailed    = "failed"
)

type ReviewerInfo struct {
	Name  string `json:"name"`
	Impl  string `json:"impl"`
	Model string `json:"model,omitempty"`
}

type ReviewStatus struct {
	ReviewID      string        `json:"review_id"`
	Project       string        `json:"project"`
	Rubric        string        `json:"rubric,omitempty"`
	State         string        `json:"state"`
	ChangesetKind string        `json:"changeset_kind"`
	Reviewer      ReviewerInfo  `json:"reviewer"`
	StartedAt     string        `json:"started_at"`
	UpdatedAt     string        `json:"updated_at"`
	CompletedAt   string        `json:"completed_at,omitempty"`
	StatusPath    string        `json:"status_path"`
	LogPath       string        `json:"log_path,omitempty"`
	Error         *errs.Info    `json:"error,omitempty"`
	Files         []string      `json:"files,omitempty"`
	Qualities     []QualityInfo `json:"qualities,omitempty"`
}

type QualityInfo struct {
	ID       string `json:"id"`
	Ref      string `json:"ref"`
	Blocking bool   `json:"blocking"`
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
