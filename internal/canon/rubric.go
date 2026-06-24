package canon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/michaelquigley/df/dd"
)

// DefaultRubric is the rubric name used when a caller does not request one.
const DefaultRubric = "rubric"

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

func LoadRubric(store *Store, project string, rubric string) (Rubric, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return Rubric{}, fmt.Errorf("project is required")
	}
	fileName, err := rubricFileName(rubric)
	if err != nil {
		return Rubric{}, err
	}
	path := filepath.Join(store.root, "projects", project, fileName)
	if err := ensureContained(store.root, path); err != nil {
		return Rubric{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Rubric{}, fmt.Errorf("load rubric %q for project %q: %w", strings.TrimSuffix(fileName, ".yaml"), project, err)
	}
	return ParseRubric(raw)
}

func LoadProjectRubric(store *Store, repoPath string, rubric string) (Rubric, string, error) {
	project := ProjectIdentity(repoPath)
	r, err := LoadRubric(store, project, rubric)
	if err != nil {
		return Rubric{}, project, err
	}
	if r.Project.Repo != project {
		return Rubric{}, project, fmt.Errorf("rubric project.repo mismatch for %q: expected %q, found %q", repoPath, project, r.Project.Repo)
	}
	return r, project, nil
}

// ListRubrics returns the rubric names available for a project, derived from the
// `*.yaml` files under `projects/<project>/`, sorted and without the extension.
func ListRubrics(store *Store, project string) ([]string, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, fmt.Errorf("project is required")
	}
	dir := filepath.Join(store.root, "projects", project)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list rubrics for project %q: %w", project, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		names = append(names, strings.TrimSuffix(name, ".yaml"))
	}
	sort.Strings(names)
	return names, nil
}

// rubricFileName normalizes a requested rubric name to a single-segment file
// name. An empty request resolves to the default rubric; names that escape the
// project directory are rejected.
func rubricFileName(rubric string) (string, error) {
	rubric = strings.TrimSpace(rubric)
	if rubric == "" {
		rubric = DefaultRubric
	}
	rubric = strings.TrimSuffix(rubric, ".yaml")
	if rubric == "" || strings.ContainsAny(rubric, `/\`) || strings.Contains(rubric, "..") {
		return "", fmt.Errorf("invalid rubric name %q", rubric)
	}
	return rubric + ".yaml", nil
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
