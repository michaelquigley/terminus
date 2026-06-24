package wiring

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/config"
	"github.com/michaelquigley/theharnessbody/reviewer"
	"github.com/michaelquigley/theharnessbody/reviewer/claude"
	"github.com/michaelquigley/theharnessbody/reviewer/codex"
	"github.com/michaelquigley/theharnessbody/reviewer/dummy"
	"github.com/michaelquigley/theharnessbody/reviewer/pi"
)

type ReviewerInfo struct {
	Name  string
	Impl  string
	Model string
}

// NewBroker assembles a broker from config. This is the composition root: the
// command layer calls it and hands the broker to the transports (the MCP server,
// the foreground review command), so those adapters never depend on wiring.
func NewBroker(cfg *config.Config) (*broker.Broker, error) {
	options, err := BrokerOptions(cfg)
	if err != nil {
		return nil, err
	}
	return broker.New(options), nil
}

func BrokerOptions(cfg *config.Config) (broker.Options, error) {
	r, info, err := BuildReviewer(cfg.Reviewer)
	if err != nil {
		return broker.Options{}, err
	}
	return broker.Options{
		LogDestination: cfg.LogDestination,
		ConfigPath:     cfg.ConfigPath,
		CanonPath:      cfg.CanonPath,
		Reviewer:       r,
		ReviewerInfo: broker.ReviewerInfo{
			Name:  info.Name,
			Impl:  info.Impl,
			Model: info.Model,
		},
	}, nil
}

func BuildReviewer(cfg *config.ReviewerConfig) (reviewer.Reviewer, ReviewerInfo, error) {
	if cfg == nil {
		return nil, ReviewerInfo{}, errors.New("reviewer is required")
	}
	info := ReviewerInfo{Name: cfg.Name, Impl: cfg.Impl, Model: cfg.Model}
	switch cfg.Impl {
	case "codex":
		return codex.New(codex.Options{
			BinaryPath: cfg.BinaryPath,
			Model:      cfg.Model,
			ExtraArgs:  append([]string(nil), cfg.ExtraArgs...),
			Env:        append([]string(nil), cfg.Env...),
		}), info, nil
	case "claude":
		return claude.New(claude.Options{
			BinaryPath: cfg.BinaryPath,
			Model:      cfg.Model,
			ExtraArgs:  append([]string(nil), cfg.ExtraArgs...),
		}), info, nil
	case "pi":
		return pi.New(pi.Options{
			BinaryPath: cfg.BinaryPath,
			Model:      cfg.Model,
			ExtraArgs:  append([]string(nil), cfg.ExtraArgs...),
		}), info, nil
	case "dummy":
		return dummy.New(dummy.Options{
			Raw: json.RawMessage(`{"summary":"dummy reviewer","findings":[]}`),
		}), info, nil
	default:
		return nil, ReviewerInfo{}, fmt.Errorf("reviewer %q: unknown impl %q", cfg.Name, cfg.Impl)
	}
}
