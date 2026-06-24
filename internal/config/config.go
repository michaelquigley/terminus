package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/michaelquigley/df/dd"
)

const DefaultLogDestination = "~/.local/share/terminus"

var knownImpls = map[string]struct{}{
	"codex":  {},
	"claude": {},
	"pi":     {},
	"dummy":  {},
}

type Config struct {
	ConfigPath     string
	CanonPath      string          `dd:"canon_path"`
	LogDestination string          `dd:"log_destination"`
	Reviewer       *ReviewerConfig `dd:"reviewer"`
}

type ReviewerConfig struct {
	Name       string   `dd:"name"`
	Impl       string   `dd:"impl"`
	BinaryPath string   `dd:"binary_path"`
	Model      string   `dd:"model"`
	ExtraArgs  []string `dd:"extra_args"`
	Env        []string `dd:"env"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		LogDestination: DefaultLogDestination,
		Reviewer: &ReviewerConfig{
			Name: "pi",
			Impl: "pi",
		},
	}
	baseDir, configPath, err := mergeCascade(cfg, path)
	if err != nil {
		return nil, err
	}
	if err := cfg.resolve(baseDir); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.ConfigPath = configPath
	return cfg, nil
}

func mergeCascade(cfg *Config, explicit string) (baseDir string, configPath string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	baseDir = cwd

	for _, candidate := range cascadePaths(explicit) {
		if candidate == "" {
			continue
		}
		expanded := expandHome(candidate)
		raw, err := os.ReadFile(expanded)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && candidate != explicit {
				continue
			}
			return "", "", err
		}
		if err := dd.MergeYAML(cfg, raw); err != nil {
			return "", "", fmt.Errorf("parse config %s: %w", expanded, err)
		}
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return "", "", err
		}
		configPath = abs
		baseDir = filepath.Dir(abs)
	}

	return baseDir, configPath, nil
}

func cascadePaths(explicit string) []string {
	paths := []string{"~/.config/terminus/config.yaml", "./terminus.yaml"}
	if explicit != "" {
		paths = append(paths, explicit)
	}
	return paths
}

func (c *Config) Validate() error {
	if c.CanonPath == "" {
		return errors.New("canon_path is required")
	}
	if c.LogDestination == "" {
		return errors.New("log_destination is required")
	}
	if c.Reviewer == nil {
		return errors.New("reviewer is required")
	}
	if c.Reviewer.Name == "" {
		return errors.New("reviewer name is required")
	}
	if c.Reviewer.Impl == "" {
		return fmt.Errorf("reviewer %q: impl is required", c.Reviewer.Name)
	}
	if _, ok := knownImpls[c.Reviewer.Impl]; !ok {
		return fmt.Errorf("reviewer %q: unknown impl %q (known: %s)", c.Reviewer.Name, c.Reviewer.Impl, strings.Join(KnownImpls(), ", "))
	}
	return nil
}

func (c *Config) EnsureLogDestination() error {
	if err := ensureDir(c.LogDestination); err != nil {
		return fmt.Errorf("log_destination: %w", err)
	}
	return nil
}

func KnownImpls() []string {
	impls := make([]string, 0, len(knownImpls))
	for impl := range knownImpls {
		impls = append(impls, impl)
	}
	slices.Sort(impls)
	return impls
}

func (c *Config) resolve(baseDir string) error {
	c.CanonPath = resolvePath(baseDir, c.CanonPath)
	c.LogDestination = resolvePath(baseDir, c.LogDestination)
	if c.Reviewer != nil && c.Reviewer.BinaryPath != "" {
		c.Reviewer.BinaryPath = resolvePath(baseDir, c.Reviewer.BinaryPath)
	}
	return nil
}

func resolvePath(baseDir string, path string) string {
	if path == "" {
		return path
	}
	path = expandHome(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func ensureDir(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", path)
		}
		return checkWritable(path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return checkWritable(path)
}

func checkWritable(dir string) error {
	file, err := os.CreateTemp(dir, ".terminus-write-*")
	if err != nil {
		return fmt.Errorf("directory %q is not writable: %w", dir, err)
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}
