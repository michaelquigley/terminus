package canon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	root string
}

func NewStore(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("canon root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: filepath.Clean(abs)}, nil
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) Load(ref string) (Quality, error) {
	cleanRef, err := CleanRef(ref)
	if err != nil {
		return Quality{}, err
	}
	path := filepath.Join(s.root, filepath.FromSlash(cleanRef)+".md")
	if err := ensureContained(s.root, path); err != nil {
		return Quality{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Quality{}, fmt.Errorf("load quality %q: %w", cleanRef, err)
	}
	q, err := ParseQuality(raw)
	if err != nil {
		return Quality{}, fmt.Errorf("load quality %q: %w", cleanRef, err)
	}
	q.Ref = cleanRef
	return q, nil
}

func CleanRef(ref string) (string, error) {
	ref = strings.TrimSpace(filepath.ToSlash(ref))
	ref = strings.TrimSuffix(ref, ".md")
	if ref == "" {
		return "", fmt.Errorf("quality ref is required")
	}
	if filepath.IsAbs(ref) || strings.HasPrefix(ref, "/") {
		return "", fmt.Errorf("quality ref %q must be canon-relative", ref)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(ref)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("quality ref %q escapes the canon", ref)
	}
	return clean, nil
}

func ensureContained(root string, path string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path %q escapes canon root %q", path, root)
	}
	return nil
}
