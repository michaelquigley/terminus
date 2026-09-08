package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/changeset"
	"github.com/michaelquigley/terminus/internal/coverage"
	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/michaelquigley/terminus/internal/monitor"
	"github.com/michaelquigley/terminus/internal/report"
	"github.com/michaelquigley/theharnessbody/reviewer/dummy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func dummyBroker(t *testing.T) *broker.Broker {
	t.Helper()
	return broker.New(broker.Options{
		LogDestination: t.TempDir(),
		Reviewer:       dummy.New(dummy.Options{Raw: json.RawMessage(`{"summary":"x","findings":[]}`)}),
		ReviewerInfo:   broker.ReviewerInfo{Name: "dummy", Impl: "dummy"},
	})
}

func dialMCP(t *testing.T, b *broker.Broker, extra ...func(*mcp.Server)) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server, err := New(b)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	for _, add := range extra {
		add(server)
	}
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// structuredContent unwraps a tool result's StructuredContent to the dd map it
// round-tripped to, so tests assert the real wire shape rather than a Go DTO.
func structuredContent(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is %T, want map[string]any", res.StructuredContent)
	}
	return m
}

// a test-only tool registered through the shared adapter over the audit DTO and
// schema. it exists before the stage-3 audit service so the requested-empty
// versus omitted coverage_map behavior is pinned through a real MCP connection.
type probeInput struct {
	IncludeMap bool
}

func probeAuditTool() tool[probeInput, report.AuditResponse] {
	return tool[probeInput, report.AuditResponse]{
		name:        "probe_audit",
		description: "test-only: exercises the shared dd adapter over the audit DTO",
		input:       objSchema(map[string]*jsonschema.Schema{"include_map": boolSchema()}),
		output:      auditResponseSchema,
		run: func(_ context.Context, in probeInput) (report.AuditResponse, error) {
			a := coverage.Assessment{
				Assessed:       true,
				FileCounts:     coverage.FileCounts{Total: 1, Uncovered: 1},
				LocalQualities: []coverage.QualityRef{},
				UncoveredFiles: []string{"a.go"},
				Exclusions:     []coverage.Exclusion{},
			}
			if in.IncludeMap {
				a.FileDetails = []coverage.FileDetail{}
			}
			return *report.AuditResponseFrom("probe", "rubric", a, in.IncludeMap), nil
		},
	}
}

func TestProbeAuditMapPresenceThroughMCP(t *testing.T) {
	cs := dialMCP(t, dummyBroker(t), func(s *mcp.Server) {
		if err := register(s, probeAuditTool()); err != nil {
			panic(err)
		}
	})

	// unrequested map: absent entirely, not null.
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "probe_audit"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	m := structuredContent(t, res)
	if _, present := m["coverage_map"]; present {
		t.Fatalf("unrequested coverage_map must be absent, got %#v", m["coverage_map"])
	}
	// required audit fields survive with exact snake_case keys.
	for _, k := range []string{"project", "rubric", "file_scope", "coverage", "dead_patterns"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("audit response missing %q: %#v", k, m)
		}
	}
	if m["file_scope"] != report.FileScopeFull {
		t.Fatalf("file_scope = %v", m["file_scope"])
	}

	// requested map over an empty tree: an empty array, never omitted or null.
	res2, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "probe_audit",
		Arguments: map[string]any{"include_map": true},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res2.IsError {
		t.Fatalf("unexpected error: %+v", res2)
	}
	m2 := structuredContent(t, res2)
	cm, ok := m2["coverage_map"].([]any)
	if !ok {
		t.Fatalf("requested coverage_map must be present, got %#v", m2["coverage_map"])
	}
	if len(cm) != 0 {
		t.Fatalf("requested-empty coverage_map = %#v, want []", cm)
	}
}

// schema discovery through a real connection: tools/list exposes the tools with
// their explicit input and output schemas.
func TestToolDiscoveryThroughMCP(t *testing.T) {
	cs := dialMCP(t, dummyBroker(t))
	list, err := cs.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range list.Tools {
		byName[tl.Name] = tl
	}
	start, ok := byName["start_review"]
	if !ok || start.InputSchema == nil || start.OutputSchema == nil {
		t.Fatalf("start_review missing or without schemas: %#v", byName["start_review"])
	}
	collect, ok := byName["collect_review"]
	if !ok || collect.InputSchema == nil || collect.OutputSchema == nil {
		t.Fatalf("collect_review missing or without schemas: %#v", byName["collect_review"])
	}
	// the start_review input schema advertises repo_path and the ad-hoc fields.
	inputObj, ok := start.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("start input schema is %T", start.InputSchema)
	}
	props, _ := inputObj["properties"].(map[string]any)
	for _, k := range []string{"repo_path", "changeset_kind", "rubric", "qualities", "qualities_blocking"} {
		if _, ok := props[k]; !ok {
			t.Fatalf("start input schema missing %q: %#v", k, props)
		}
	}
}

// input validation runs against the decoded map before any defaulting or
// binding: a wrong type and an unknown field each fail as user_error.
func TestInputValidationThroughMCP(t *testing.T) {
	cs := dialMCP(t, dummyBroker(t))

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "start_review",
		Arguments: map[string]any{"repo_path": 123},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected a tool error for a non-string repo_path")
	}
	m := structuredContent(t, res)
	if m["error"].(map[string]any)["code"] != errs.CodeUserError {
		t.Fatalf("expected user_error, got %#v", m["error"])
	}

	res2, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "start_review",
		Arguments: map[string]any{"repo_path": ".", "bogus": true},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res2.IsError {
		t.Fatal("expected a tool error for an unknown field")
	}
	m2 := structuredContent(t, res2)
	if m2["error"].(map[string]any)["code"] != errs.CodeUserError {
		t.Fatalf("expected user_error for unknown field, got %#v", m2["error"])
	}
}

// a classified service error (unknown review) flows back as an IsError tool
// result with the exact dd-unbound {error:{code,message,details}} envelope.
func TestStructuredErrorThroughMCP(t *testing.T) {
	cs := dialMCP(t, dummyBroker(t))
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "collect_review",
		Arguments: map[string]any{"review_id": "nope"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError result for unknown review_id")
	}
	m := structuredContent(t, res)
	inner, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope missing: %#v", m)
	}
	if inner["code"] != errs.CodeNotFound {
		t.Fatalf("code = %v, want not_found", inner["code"])
	}
	if _, ok := inner["message"]; !ok {
		t.Fatalf("message missing: %#v", inner)
	}
	if _, ok := inner["details"]; !ok {
		t.Fatalf("details missing: %#v", inner)
	}
}

// a real review collected over MCP returns a dd-unbound map whose raw reviewer
// output is a nested object (not a byte array), whose collections are arrays
// (never null), and whose keys are exact snake_case.
func TestReviewCollectThroughMCP(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")

	canonRoot := fixtureCanon(t, filepath.Base(repo))
	raw := json.RawMessage(`{"summary":"one finding","findings":[{"id":"f1","quality":"df-logging","file":"main.go","lines":"1","claim":"c","rationale":"r","suggestion":null}]}`)
	b := broker.New(broker.Options{
		LogDestination: t.TempDir(),
		CanonPath:      canonRoot,
		Reviewer:       dummy.New(dummy.Options{Raw: raw}),
		ReviewerInfo:   broker.ReviewerInfo{Name: "dummy", Impl: "dummy"},
	})

	cs := dialMCP(t, b)
	startRes, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "start_review",
		Arguments: map[string]any{"repo_path": repo},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if startRes.IsError {
		t.Fatalf("start error: %+v", startRes)
	}
	startMap := structuredContent(t, startRes)
	reviewID, _ := startMap["review_id"].(string)
	if reviewID == "" {
		t.Fatalf("start response missing review_id: %#v", startMap)
	}

	// poll collect until the review completes.
	var collectMap map[string]any
	deadline := time.Now().Add(3 * time.Second)
	for {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "collect_review",
			Arguments: map[string]any{"review_id": reviewID},
		})
		if err != nil {
			t.Fatalf("collect: %v", err)
		}
		if !res.IsError {
			collectMap = structuredContent(t, res)
			break
		}
		if code := collectErrorCode(res); code != errs.CodeConflict || time.Now().After(deadline) {
			t.Fatalf("collect failed: %#v", res.StructuredContent)
		}
		time.Sleep(20 * time.Millisecond)
	}

	review, ok := collectMap["review"].(map[string]any)
	if !ok {
		t.Fatalf("collect response missing review object: %#v", collectMap)
	}
	// raw reviewer output is a nested object, not a number array.
	ra, ok := review["raw"].(map[string]any)
	if !ok {
		t.Fatalf("raw is %T, want nested object", review["raw"])
	}
	if ra["summary"] != "one finding" {
		t.Fatalf("raw summary = %v", ra["summary"])
	}
	// findings is a non-empty array.
	if fs, ok := review["findings"].([]any); !ok || len(fs) != 1 {
		t.Fatalf("findings = %#v, want one finding", review["findings"])
	}
	// excluded_qualities was omitted when empty before the dd migration; the
	// restored +omitempty keeps historical and new records on the same rule.
	if _, present := review["excluded_qualities"]; present {
		t.Fatalf("empty excluded_qualities must be absent: %#v", review["excluded_qualities"])
	}
	// the single-review branch omits the list branch: presence distinguishes them.
	if _, present := collectMap["reviews"]; present {
		t.Fatalf("collect branch must omit reviews: %#v", collectMap["reviews"])
	}

	// the list branch (no review_id) is the mirror image: reviews present,
	// review absent.
	listRes, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "collect_review"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("list error: %+v", listRes)
	}
	listMap := structuredContent(t, listRes)
	if _, present := listMap["review"]; present {
		t.Fatalf("list branch must omit review: %#v", listMap)
	}
	ls, ok := listMap["reviews"].([]any)
	if !ok || len(ls) != 1 {
		t.Fatalf("list branch reviews = %#v, want the one completed review", listMap["reviews"])
	}

	// stage 2 will populate coverage on review results; stage 1 must not
	// manufacture a coverage object, so the field is absent (historical nil).
	if _, present := review["coverage"]; present {
		t.Fatalf("stage 1 must not populate coverage on review results: %#v", review["coverage"])
	}
}

func collectErrorCode(res *mcp.CallToolResult) string {
	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		return ""
	}
	inner, ok := m["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := inner["code"].(string)
	return code
}

// the raw-reviewer-JSON passthrough is shared by disk and MCP: a result the
// broker wrote must collect identically from disk through a second broker.
func TestRawJSONSharedCodecRoundTrip(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")

	canonRoot := fixtureCanon(t, filepath.Base(repo))
	raw := json.RawMessage(`{"summary":"clean","findings":[]}`)
	opts := broker.Options{
		LogDestination: t.TempDir(),
		CanonPath:      canonRoot,
		Reviewer:       dummy.New(dummy.Options{Raw: raw}),
		ReviewerInfo:   broker.ReviewerInfo{Name: "dummy", Impl: "dummy"},
	}
	live, err := broker.New(opts).RunReview(context.Background(), broker.StartReviewRequest{RepoPath: repo, ChangesetKind: changeset.KindWorkingTree})
	if err != nil {
		t.Fatal(err)
	}
	// a second broker reads from disk through the shared report codec.
	got, err := broker.New(opts).CollectReview(context.Background(), broker.CollectReviewRequest{Project: live.Project, ReviewID: live.ReviewID})
	if err != nil {
		t.Fatalf("collect from disk: %v", err)
	}
	var liveRaw, gotRaw any
	if err := json.Unmarshal(live.Raw, &liveRaw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got.Raw, &gotRaw); err != nil {
		t.Fatalf("disk Raw is not valid JSON: %v (%s)", err, got.Raw)
	}
	if !reflect.DeepEqual(liveRaw, gotRaw) {
		t.Fatalf("raw drifted through the shared codec:\n disk: %s\n live: %s", got.Raw, live.Raw)
	}
}

// the dd output of the success DTOs must validate against the declared schemas,
// so discovery, validation, and serialization cannot drift apart.
func TestSchemasAgreeWithDDOutput(t *testing.T) {
	cases := []struct {
		name   string
		out    any
		schema *jsonschema.Schema
	}{
		{
			name:   "audit",
			out:    &report.AuditResponse{Project: "p", Rubric: "rubric", FileScope: report.FileScopeFull, Coverage: &report.Coverage{Assessed: true, FileCounts: &report.FileCounts{Total: 1, Uncovered: 1}, LocalQualities: []report.QualityRef{}, UncoveredFiles: []string{"a.go"}, Exclusions: []report.Exclusion{}}, DeadPatterns: []report.DeadPattern{}},
			schema: auditResponseSchema,
		},
		{
			name:   "coverage-assessed",
			out:    &report.Coverage{Assessed: true, FileCounts: &report.FileCounts{Total: 2, Excluded: 1, Covered: 1}, LocalQualities: []report.QualityRef{{ID: "q", Ref: "projects/p/q"}}, UncoveredFiles: []string{"a.go"}, Exclusions: []report.Exclusion{{Pattern: "docs/**", Files: []string{"docs/x.md"}}}},
			schema: coverageSchema(),
		},
		{
			name:   "coverage-adhoc",
			out:    &report.Coverage{Assessed: false, Reason: "ad_hoc", LocalQualities: []report.QualityRef{}, UncoveredFiles: []string{}, Exclusions: []report.Exclusion{}},
			schema: coverageSchema(),
		},
		{
			name:   "review-summary",
			out:    &broker.ReviewSummary{ReviewID: "r1", Project: "p", State: "completed", ChangesetKind: "working-tree", StartedAt: "2026-01-01T00:00:00Z", StatusPath: "/s"},
			schema: reviewSummarySchema(),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := report.Unbind(c.out)
			if err != nil {
				t.Fatalf("unbind: %v", err)
			}
			r, err := resolveSchema(c.name, c.schema)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if verr := r.Validate(data); verr != nil {
				t.Fatalf("dd output failed its schema: %v\n%#v", verr, data)
			}
		})
	}
}

// project-owned DTOs must not carry json tags; dd owns all payload binding.
func TestNoJSONTagsOnProjectDTOs(t *testing.T) {
	types := []any{
		report.Coverage{}, report.FileCounts{}, report.QualityRef{}, report.Exclusion{},
		report.DeadPattern{}, report.MapFile{}, report.MapGroup{}, report.AuditResponse{},
		broker.CollectReviewResponse{}, broker.ReviewSummary{}, broker.ExcludedQuality{},
		broker.TriageFindingOutput{}, broker.StartReviewResponse{}, broker.ListReviewsResponse{},
		monitor.ReviewStatus{}, monitor.ReviewerInfo{}, monitor.QualityInfo{},
		errs.Info{}, StartReviewInput{}, StartReviewOutput{}, CollectReviewInput{},
		CollectReviewOutput{}, ToolErrorOutput{}, ErrorOutput{},
	}
	for _, v := range types {
		checkNoJSONTags(t, reflect.ValueOf(v), reflect.TypeOf(v).Name())
	}
}

func checkNoJSONTags(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return
	}
	tv := v.Type()
	for i := 0; i < tv.NumField(); i++ {
		f := tv.Field(i)
		if tag := f.Tag.Get("json"); tag != "" && tag != "-" {
			t.Errorf("%s.%s carries json tag %q; project DTOs must bind through dd only", path, f.Name, tag)
		}
		if f.Type.Kind() == reflect.Struct || f.Type.Kind() == reflect.Pointer {
			checkNoJSONTags(t, v.Field(i), path+"."+f.Name)
		}
	}
}

// git + canon fixtures, mirroring the broker test helpers.

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test User")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixtureCanon(t *testing.T, project string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go-conventions", "df-logging.md"), `---
id: df-logging
territory:
  - "**/*.go"
---
# df logging
`)
	writeFile(t, filepath.Join(root, "projects", project, "rubric.yaml"), fmt.Sprintf(`project:
  repo: %q
qualities:
  - ref: go-conventions/df-logging
    blocking: true
`, project))
	return root
}
