package changeset

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/michaelquigley/theharnessbody/repo"
	"github.com/michaelquigley/theharnessbody/scope"
)

const (
	KindWorkingTree = "working-tree"
	KindPaths       = "paths"
	KindFull        = "full"
)

type Changeset struct {
	Kind  string
	Files []string
	Diff  string
}

func WorkingTree(ctx context.Context, repoPath string) (Changeset, error) {
	g, err := repo.New(repoPath, "")
	if err != nil {
		return Changeset{}, err
	}
	status, err := g.Status(ctx)
	if err != nil {
		return Changeset{}, err
	}
	files := append([]string{}, status.Modified...)
	files = append(files, status.Added...)
	files = append(files, status.Deleted...)
	files = append(files, status.Untracked...)
	diff, err := g.Diff()
	if err != nil {
		return Changeset{}, err
	}
	return Changeset{
		Kind:  KindWorkingTree,
		Files: normalizeFiles(files),
		Diff:  diff,
	}, nil
}

func Paths(ctx context.Context, repoPath string, paths []string) (Changeset, error) {
	if len(paths) == 0 {
		return Changeset{}, fmt.Errorf("paths changeset requires at least one path")
	}
	resolved, err := scope.Resolve(ctx, repoPath, scope.Spec{Type: scope.KindPaths, Paths: paths}, time.Time{})
	if err != nil {
		return Changeset{}, err
	}
	return Changeset{
		Kind:  KindPaths,
		Files: normalizeFiles(resolved.Files),
	}, nil
}

func Full(ctx context.Context, repoPath string) (Changeset, error) {
	resolved, err := scope.Resolve(ctx, repoPath, scope.Spec{Type: scope.KindFull}, time.Time{})
	if err != nil {
		return Changeset{}, err
	}
	return Changeset{
		Kind:  KindFull,
		Files: normalizeFiles(resolved.Files),
	}, nil
}

func normalizeFiles(files []string) []string {
	seen := map[string]struct{}{}
	for _, file := range files {
		file = filepath.ToSlash(strings.TrimSpace(file))
		file = strings.TrimPrefix(file, "./")
		file = strings.Trim(file, "/")
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
