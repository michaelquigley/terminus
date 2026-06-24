package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRootCommandShowsHelp(t *testing.T) {
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("root command failed: %v\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{"Usage:", "terminus [flags]", "review", "serve"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help output missing %q\n%s", want, text)
		}
	}
}
