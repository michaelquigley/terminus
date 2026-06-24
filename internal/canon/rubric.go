package canon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/df/dd"
)

type ProjectInfo struct {
	Name            string         `dd:"name"`
	Repo            string         `dd:"repo,+required"`
	Description     string         `dd:"description"`
	Characteristics []string       `dd:"characteristics"`
	Extra           map[string]any `dd:",+extra"`
}

type RubricEntry struct {
	Ref      string         `dd:"ref,+required"`
	Blocking bool           `dd:"blocking"`
	Extra    map[string]any `dd:",+extra"`
}

type Rubric struct {
	Project   ProjectInfo    `dd:"project"`
	Qualities []RubricEntry  `dd:"qualities,+required"`
	Extra     map[string]any `dd:",+extra"`
}

func LoadRubric(store *Store, project string) (Rubric, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return Rubric{}, fmt.Errorf("project is required")
	}
	path := filepath.Join(store.root, "projects", project, "rubric.yaml")
	if err := ensureContained(store.root, path); err != nil {
		return Rubric{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Rubric{}, fmt.Errorf("load rubric %q: %w", project, err)
	}
	return ParseRubric(raw)
}

func LoadProjectRubric(store *Store, repoPath string) (Rubric, string, error) {
	project := ProjectIdentity(repoPath)
	r, err := LoadRubric(store, project)
	if err != nil {
		return Rubric{}, project, err
	}
	if r.Project.Repo != project {
		return Rubric{}, project, fmt.Errorf("rubric project.repo mismatch for %q: expected %q, found %q", repoPath, project, r.Project.Repo)
	}
	return r, project, nil
}

func ParseRubric(raw []byte) (Rubric, error) {
	var r Rubric
	if err := dd.MergeYAML(&r, raw); err != nil {
		return Rubric{}, fmt.Errorf("parse rubric: %w", err)
	}
	if err := rejectExtra("rubric", r.Extra); err != nil {
		return Rubric{}, err
	}
	if err := rejectExtra("rubric project", r.Project.Extra); err != nil {
		return Rubric{}, err
	}
	for i, entry := range r.Qualities {
		if err := rejectExtra(fmt.Sprintf("rubric qualities[%d]", i), entry.Extra); err != nil {
			return Rubric{}, err
		}
		clean, err := CleanRef(entry.Ref)
		if err != nil {
			return Rubric{}, fmt.Errorf("rubric qualities[%d]: %w", i, err)
		}
		r.Qualities[i].Ref = clean
	}
	return r, nil
}

func ProjectIdentity(repoPath string) string {
	clean := filepath.Clean(repoPath)
	return filepath.Base(clean)
}
