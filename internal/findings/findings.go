package findings

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/michaelquigley/terminus/internal/canon"
	"github.com/michaelquigley/theharnessbody/reviewer/schema"
)

//go:embed schema.json
var schemaFS embed.FS

type Output struct {
	Summary  string    `json:"summary"`
	Findings []Finding `json:"findings"`
}

type Finding struct {
	ID         string  `json:"id"`
	Quality    string  `json:"quality"`
	File       string  `json:"file"`
	Lines      string  `json:"lines"`
	Claim      string  `json:"claim"`
	Rationale  string  `json:"rationale"`
	Suggestion *string `json:"suggestion"`
}

type Classified struct {
	Finding  Finding `json:"finding"`
	Blocking bool    `json:"blocking"`
}

func Schema() json.RawMessage {
	data, err := schemaFS.ReadFile("schema.json")
	if err != nil {
		panic(fmt.Sprintf("read embedded schema: %v", err))
	}
	return append(json.RawMessage(nil), data...)
}

func Validate(raw json.RawMessage) error {
	if err := schema.Validate(raw, Schema()); err != nil {
		return err
	}
	out, err := Parse(raw)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, f := range out.Findings {
		id := strings.TrimSpace(f.ID)
		if id == "" {
			return fmt.Errorf("finding id is empty")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate finding id %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func Parse(raw json.RawMessage) (Output, error) {
	var out Output
	if err := json.Unmarshal(raw, &out); err != nil {
		return Output{}, fmt.Errorf("parse reviewer output: %w", err)
	}
	return out, nil
}

func CheckAttribution(fs []Finding, selected []canon.Selected) error {
	ids := selectedIDs(selected)
	for _, f := range fs {
		if _, ok := ids[f.Quality]; !ok {
			return fmt.Errorf("finding %q references unknown quality %q (selected: %s)", f.ID, f.Quality, strings.Join(sortedKeys(ids), ", "))
		}
	}
	return nil
}

func Classify(fs []Finding, selected []canon.Selected) []Classified {
	blocking := map[string]bool{}
	for _, s := range selected {
		blocking[s.Quality.Head.ID] = s.Blocking
	}
	out := make([]Classified, 0, len(fs))
	for _, f := range fs {
		out = append(out, Classified{
			Finding:  f,
			Blocking: blocking[f.Quality],
		})
	}
	return out
}

func selectedIDs(selected []canon.Selected) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, s := range selected {
		ids[s.Quality.Head.ID] = struct{}{}
	}
	return ids
}

func sortedKeys[K comparable](m map[K]struct{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, fmt.Sprint(key))
	}
	sort.Strings(keys)
	return keys
}
