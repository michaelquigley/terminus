package wiring

import (
	"testing"

	"github.com/michaelquigley/terminus/internal/config"
)

func TestBuildReviewerImpls(t *testing.T) {
	for _, impl := range []string{"codex", "claude", "pi", "dummy"} {
		r, info, err := BuildReviewer(&config.ReviewerConfig{Name: "r", Impl: impl})
		if err != nil {
			t.Fatalf("impl %q: %v", impl, err)
		}
		if r == nil {
			t.Fatalf("impl %q: nil reviewer", impl)
		}
		if info.Impl != impl || info.Name != "r" {
			t.Fatalf("impl %q: unexpected info %#v", impl, info)
		}
	}
	if _, _, err := BuildReviewer(&config.ReviewerConfig{Name: "r", Impl: "bogus"}); err == nil {
		t.Fatal("expected unknown impl error")
	}
	if _, _, err := BuildReviewer(nil); err == nil {
		t.Fatal("expected error for nil reviewer config")
	}
}

func TestNewBroker(t *testing.T) {
	cfg := &config.Config{
		CanonPath:      "/tmp/canon",
		LogDestination: "/tmp/logs",
		Reviewer:       &config.ReviewerConfig{Name: "dummy", Impl: "dummy"},
	}
	b, err := NewBroker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil {
		t.Fatal("nil broker")
	}
}
