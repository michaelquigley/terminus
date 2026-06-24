package broker

import (
	"testing"

	"github.com/michaelquigley/terminus/internal/findings"
)

func TestClean(t *testing.T) {
	clean, blocking := Clean([]findings.Classified{
		{Finding: findings.Finding{ID: "a"}, Blocking: false},
	})
	if !clean || len(blocking) != 0 {
		t.Fatalf("expected clean advisory-only review")
	}

	clean, blocking = Clean([]findings.Classified{
		{Finding: findings.Finding{ID: "a"}, Blocking: true},
	})
	if clean || len(blocking) != 1 {
		t.Fatalf("expected blocking review to be not clean")
	}
}
