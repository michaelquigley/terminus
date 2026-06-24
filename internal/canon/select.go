package canon

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type Selected struct {
	Quality  Quality
	Blocking bool
}

func Compose(store *Store, r Rubric) ([]Selected, error) {
	selected := make([]Selected, 0, len(r.Qualities))
	ids := map[string]string{}
	for _, entry := range r.Qualities {
		q, err := store.Load(entry.Ref)
		if err != nil {
			return nil, err
		}
		if previous, ok := ids[q.Head.ID]; ok {
			return nil, fmt.Errorf("duplicate quality id %q in refs %q and %q", q.Head.ID, previous, q.Ref)
		}
		ids[q.Head.ID] = q.Ref
		for _, territory := range q.Head.Territory {
			if err := ValidateTerritory(territory); err != nil {
				return nil, fmt.Errorf("quality %q territory %q: %w", q.Ref, territory, err)
			}
		}
		selected = append(selected, Selected{Quality: q, Blocking: entry.Blocking})
	}
	return selected, nil
}

func Narrow(composed []Selected, changedFiles []string) []Selected {
	files := normalizeFiles(changedFiles)
	out := make([]Selected, 0, len(composed))
	for _, s := range composed {
		if len(s.Quality.Head.Territory) == 0 {
			out = append(out, s)
			continue
		}
		if territoryMatchesAny(s.Quality.Head.Territory, files) {
			out = append(out, s)
		}
	}
	return out
}

func ValidateTerritory(pattern string) error {
	pattern = normalizePattern(pattern)
	if pattern == "" {
		return fmt.Errorf("empty territory pattern")
	}
	_, err := matchTerritory(pattern, "__terminus_validation__/file.go")
	return err
}

func territoryMatchesAny(patterns []string, files []string) bool {
	for _, pattern := range patterns {
		for _, file := range files {
			matched, err := matchTerritory(pattern, file)
			if err == nil && matched {
				return true
			}
		}
	}
	return false
}

func matchTerritory(pattern string, file string) (bool, error) {
	pattern = normalizePattern(pattern)
	file = normalizeFile(file)
	if pattern == "" || file == "" {
		return false, nil
	}
	patternSegments := strings.Split(pattern, "/")
	fileSegments := strings.Split(file, "/")
	return matchSegments(patternSegments, fileSegments)
}

func matchSegments(patterns []string, files []string) (bool, error) {
	if len(patterns) == 0 {
		return len(files) == 0, nil
	}
	p := patterns[0]
	if strings.Contains(p, "**") && p != "**" {
		return false, fmt.Errorf("** must occupy its own path segment")
	}
	if p == "**" {
		if len(patterns) == 1 {
			return true, nil
		}
		for i := 0; i <= len(files); i++ {
			matched, err := matchSegments(patterns[1:], files[i:])
			if err != nil || matched {
				return matched, err
			}
		}
		return false, nil
	}
	if len(files) == 0 {
		return false, nil
	}
	matched, err := path.Match(p, files[0])
	if err != nil {
		return false, err
	}
	if !matched {
		return false, nil
	}
	return matchSegments(patterns[1:], files[1:])
}

func normalizeFiles(files []string) []string {
	seen := map[string]struct{}{}
	for _, file := range files {
		file = normalizeFile(file)
		if file != "" {
			seen[file] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for file := range seen {
		out = append(out, file)
	}
	sort.Strings(out)
	return out
}

func normalizeFile(file string) string {
	file = filepath.ToSlash(strings.TrimSpace(file))
	file = strings.TrimPrefix(file, "./")
	file = strings.Trim(file, "/")
	return file
}

func normalizePattern(pattern string) string {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	pattern = strings.TrimPrefix(pattern, "./")
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	return strings.Trim(pattern, "/")
}
