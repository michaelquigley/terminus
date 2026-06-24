package canon

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/michaelquigley/df/dd"
)

type Head struct {
	ID         string         `dd:"id,+required"`
	AppliesTo  []string       `dd:"applies_to"`
	Territory  []string       `dd:"territory"`
	Convention string         `dd:"convention"`
	Extra      map[string]any `dd:",+extra"`
}

type Quality struct {
	Head Head
	Body string
	Ref  string
}

func ParseQuality(data []byte) (Quality, error) {
	head, body, err := splitFrontmatter(data)
	if err != nil {
		return Quality{}, err
	}
	var h Head
	if err := dd.MergeYAML(&h, head); err != nil {
		return Quality{}, fmt.Errorf("parse quality head: %w", err)
	}
	if err := rejectExtra("quality head", h.Extra); err != nil {
		return Quality{}, err
	}
	return Quality{Head: h, Body: strings.TrimLeft(string(body), "\r\n")}, nil
}

func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return nil, nil, fmt.Errorf("quality is missing YAML frontmatter")
	}
	lines := bytes.SplitAfter(data, []byte("\n"))
	var end int
	offset := len(lines[0])
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(string(lines[i]))
		if line == "---" {
			end = offset
			bodyOffset := offset + len(lines[i])
			return data[len(lines[0]):end], data[bodyOffset:], nil
		}
		offset += len(lines[i])
	}
	return nil, nil, fmt.Errorf("quality frontmatter is not closed")
}

func rejectExtra(where string, extra map[string]any) error {
	if len(extra) == 0 {
		return nil
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Errorf("%s has unknown key %q", where, keys[0])
}
