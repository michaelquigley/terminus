package findings

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/michaelquigley/terminus/internal/canon"
	"github.com/michaelquigley/theharnessbody/reviewer/schema"
)

func TestSchemaGuardEnvelope(t *testing.T) {
	if err := schema.GuardEnvelope(Schema()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsDuplicateIDs(t *testing.T) {
	raw := json.RawMessage(`{"summary":"x","findings":[
		{"id":"f1","quality":"q","file":"a.go","lines":"1","claim":"c","rationale":"r","suggestion":null},
		{"id":"f1","quality":"q","file":"a.go","lines":"2","claim":"c","rationale":"r","suggestion":null}
	]}`)
	err := Validate(raw)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestCheckAttributionAndScope(t *testing.T) {
	selected := []canon.Selected{
		{Quality: canon.Quality{Head: canon.Head{ID: "df-logging"}, Ref: "go/df"}, Blocking: true},
	}
	fs := []Finding{{ID: "f1", Quality: "other", File: "main.go"}}
	if err := CheckAttribution(fs, selected); err == nil {
		t.Fatal("expected unknown quality error")
	}

	fs = []Finding{{ID: "f1", Quality: "df-logging", File: "internal/other.go"}}
	if err := CheckScope(fs, []string{"main.go"}); err == nil {
		t.Fatal("expected scope error")
	}

	fs = []Finding{{ID: "f1", Quality: "df-logging", File: "./main.go"}}
	if err := CheckAttribution(fs, selected); err != nil {
		t.Fatal(err)
	}
	if err := CheckScope(fs, []string{"main.go"}); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyUsesSelectedQualityBlocking(t *testing.T) {
	selected := []canon.Selected{
		{Quality: canon.Quality{Head: canon.Head{ID: "df-logging"}, Ref: "go/df"}, Blocking: true},
		{Quality: canon.Quality{Head: canon.Head{ID: "truthful-naming"}, Ref: "go/name"}, Blocking: false},
	}
	classified := Classify([]Finding{
		{ID: "f1", Quality: "df-logging"},
		{ID: "f2", Quality: "truthful-naming"},
	}, selected)
	if !classified[0].Blocking {
		t.Fatal("expected df-logging to block")
	}
	if classified[1].Blocking {
		t.Fatal("expected truthful-naming to be advisory")
	}
}
