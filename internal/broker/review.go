package broker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/michaelquigley/terminus/internal/canon"
	"github.com/michaelquigley/terminus/internal/changeset"
	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/michaelquigley/terminus/internal/findings"
	"github.com/michaelquigley/terminus/internal/monitor"
	terminusprompt "github.com/michaelquigley/terminus/internal/prompt"
	"github.com/michaelquigley/theharnessbody/record"
	"github.com/michaelquigley/theharnessbody/reviewer"
)

const (
	promptFileName   = "_prompt.md"
	findingsFileName = "_findings.md"
	resultFileName   = "result.json"
	adHocRubric      = "(ad-hoc)"
)

type Broker struct {
	mu      sync.Mutex
	options Options
	jobs    map[string]*reviewJob
}

type reviewJob struct {
	id           string
	project      string
	rubric       string
	repoPath     string
	dir          string
	statusPath   string
	promptPath   string
	logPath      string
	resultPath   string
	startedAt    time.Time
	changeset    changeset.Changeset
	selected     []canon.Selected
	reviewer     reviewer.Reviewer
	reviewerName string
	done         chan struct{}
	result       CollectReviewResponse
	err          error
}

func New(options Options) *Broker {
	return &Broker{
		options: options,
		jobs:    map[string]*reviewJob{},
	}
}

func (b *Broker) StartReview(ctx context.Context, req StartReviewRequest) (StartReviewResponse, error) {
	job, promptText, response, err := b.prepareReview(ctx, req)
	if err != nil {
		return StartReviewResponse{}, err
	}

	go b.execute(context.Background(), job, promptText)

	return response, nil
}

func (b *Broker) RunReview(ctx context.Context, req StartReviewRequest) (CollectReviewResponse, error) {
	job, promptText, _, err := b.prepareReview(ctx, req)
	if err != nil {
		return CollectReviewResponse{}, err
	}

	b.execute(ctx, job, promptText)

	b.mu.Lock()
	defer b.mu.Unlock()
	if job.err != nil {
		return CollectReviewResponse{}, job.err
	}
	return cloneCollectReviewResponse(job.result), nil
}

func (b *Broker) prepareReview(ctx context.Context, req StartReviewRequest) (*reviewJob, string, StartReviewResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", StartReviewResponse{}, err
	}
	if b.options.Reviewer == nil {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeInternalError, "no reviewer configured", nil, nil)
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeUserError, "repo_path is required", nil, nil)
	}
	repoPath, err := filepath.Abs(req.RepoPath)
	if err != nil {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeUserError, "resolve repo_path", err, nil)
	}

	store, err := canon.NewStore(b.options.CanonPath)
	if err != nil {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeUserError, "open canon", err, nil)
	}
	var rubric canon.Rubric
	var project string
	var rubricName string
	if len(req.Qualities) > 0 {
		// ad-hoc review: take the quality refs directly, bypassing rubric
		// resolution. Compose still loads, validates, and dedupes them.
		project = canon.ProjectIdentity(repoPath)
		rubricName = adHocRubric
		entries := make([]canon.RubricEntry, 0, len(req.Qualities))
		for _, ref := range req.Qualities {
			entries = append(entries, canon.RubricEntry{Ref: ref, Blocking: req.QualitiesBlocking})
		}
		rubric = canon.Rubric{Project: canon.ProjectInfo{Repo: project}, Qualities: entries}
	} else {
		rubricName = strings.TrimSpace(req.Rubric)
		if rubricName == "" {
			rubricName = canon.DefaultRubric
		}
		rubric, project, err = canon.LoadProjectRubric(store, repoPath, rubricName)
		if err != nil {
			return nil, "", StartReviewResponse{}, errs.New(errs.CodeUserError, "load project rubric", err, map[string]any{"repo_path": repoPath, "rubric": rubricName})
		}
	}
	cs, err := b.extractChangeset(ctx, repoPath, req)
	if err != nil {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeUserError, "extract changeset", err, nil)
	}
	composed, err := canon.Compose(store, rubric)
	if err != nil {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeUserError, "compose rubric", err, nil)
	}
	selected := canon.Narrow(composed, cs.Files)

	id, err := newReviewID()
	if err != nil {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeInternalError, "allocate review id", err, nil)
	}
	dir := monitor.ReviewDir(b.options.LogDestination, project, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeInternalError, "create review directory", err, map[string]any{"dir": dir})
	}
	promptText := terminusprompt.Build(terminusprompt.Request{
		RepoPath:  repoPath,
		Selected:  selected,
		Changeset: cs,
	})
	promptPath := filepath.Join(dir, promptFileName)
	if err := os.WriteFile(promptPath, []byte(promptText), 0o600); err != nil {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeInternalError, "write prompt", err, map[string]any{"prompt_path": promptPath})
	}

	startedAt := time.Now().UTC()
	job := &reviewJob{
		id:           id,
		project:      project,
		rubric:       rubricName,
		repoPath:     repoPath,
		dir:          dir,
		statusPath:   monitor.StatusPath(dir),
		promptPath:   promptPath,
		logPath:      filepath.Join(dir, findingsFileName),
		resultPath:   filepath.Join(dir, resultFileName),
		startedAt:    startedAt,
		changeset:    cs,
		selected:     cloneSelected(selected),
		reviewer:     b.options.Reviewer,
		reviewerName: b.options.ReviewerInfo.Name,
		done:         make(chan struct{}),
	}

	b.mu.Lock()
	b.jobs[jobKey(project, id)] = job
	b.mu.Unlock()

	if err := monitor.WriteStatus(job.statusPath, job.status(monitor.StateRunning, "", nil)); err != nil {
		return nil, "", StartReviewResponse{}, errs.New(errs.CodeInternalError, "write review status", err, map[string]any{"status_path": job.statusPath})
	}

	response := StartReviewResponse{
		ReviewID:       id,
		Project:        project,
		State:          monitor.StateRunning,
		Reviewer:       b.options.ReviewerInfo.Name,
		StartedAt:      startedAt,
		StatusPath:     job.statusPath,
		MonitorCommand: b.monitorCommand(project, id),
		NextAction:     "tell the user this review is running; they can monitor it with the monitor_command and collect it when it completes",
	}
	return job, promptText, response, nil
}

func (b *Broker) extractChangeset(ctx context.Context, repoPath string, req StartReviewRequest) (changeset.Changeset, error) {
	switch req.ChangesetKind {
	case "", changeset.KindWorkingTree:
		return changeset.WorkingTree(ctx, repoPath)
	case changeset.KindPaths:
		return changeset.Paths(ctx, repoPath, req.Paths)
	case changeset.KindFull:
		return changeset.Full(ctx, repoPath)
	default:
		return changeset.Changeset{}, fmt.Errorf("unknown changeset kind %q", req.ChangesetKind)
	}
}

func (b *Broker) execute(ctx context.Context, job *reviewJob, promptText string) {
	resp, err := job.reviewer.Review(ctx, reviewer.ReviewRequest{
		Prompt:     promptText,
		Schema:     findings.Schema(),
		WorkingDir: job.repoPath,
	})
	if err != nil {
		b.finishFailure(job, errs.New(errs.CodeReviewerFailed, "reviewer failed", err, map[string]any{"reviewer": job.reviewerName}))
		return
	}
	if err := findings.Validate(resp.Raw); err != nil {
		b.finishFailure(job, errs.New(errs.CodeReviewerFailed, "reviewer output failed schema validation", err, map[string]any{"reviewer": job.reviewerName, "raw": string(resp.Raw)}))
		return
	}
	output, err := findings.Parse(resp.Raw)
	if err != nil {
		b.finishFailure(job, errs.New(errs.CodeReviewerFailed, "reviewer output failed parse", err, map[string]any{"reviewer": job.reviewerName, "raw": string(resp.Raw)}))
		return
	}
	if err := findings.CheckAttribution(output.Findings, job.selected); err != nil {
		b.finishFailure(job, errs.New(errs.CodeReviewerFailed, "reviewer output used an unknown quality", err, map[string]any{"reviewer": job.reviewerName, "raw": string(resp.Raw)}))
		return
	}

	classified := findings.Classify(output.Findings, job.selected)
	clean, _ := Clean(classified)
	verdict := VerdictNotClean
	if clean {
		verdict = VerdictClean
	}
	triage := orderTriage(classified)
	result := CollectReviewResponse{
		ReviewID:     job.id,
		Project:      job.project,
		Rubric:       job.rubric,
		State:        monitor.StateCompleted,
		Verdict:      verdict,
		Clean:        clean,
		Summary:      output.Summary,
		LogPath:      job.logPath,
		PromptPath:   job.promptPath,
		ReviewerName: job.reviewerName,
		Raw:          append(json.RawMessage(nil), resp.Raw...),
		Findings:     triage,
		Guidance:     triageGuidance(triage),
	}
	if len(triage) > 0 {
		next := triage[0]
		result.NextFinding = &next
	}

	if err := writeRecord(job, verdict, resp, classified); err != nil {
		b.finishFailure(job, errs.New(errs.CodeInternalError, "write findings document", err, map[string]any{"log_path": job.logPath}))
		return
	}
	if err := writeResult(job.resultPath, result); err != nil {
		b.finishFailure(job, errs.New(errs.CodeInternalError, "write review result", err, map[string]any{"result_path": job.resultPath}))
		return
	}
	b.finishSuccess(job, result)
}

func writeRecord(job *reviewJob, verdict string, resp reviewer.ReviewResponse, classified []findings.Classified) error {
	return record.WriteInitial(job.logPath, record.Entry{
		SessionID:   job.id,
		RoundNumber: 1,
		OpenedAt:    job.startedAt,
		Verdict:     verdict,
		PromptPath:  filepath.Base(job.promptPath),
		Manifest: []record.ArtifactManifestEntry{{
			Name:       "changeset",
			SourcePath: job.repoPath,
			Size:       int64(len(job.changeset.Diff)),
		}},
		Reviewers: []record.ReviewerOutput{{
			Name:       job.reviewerName,
			Raw:        append(json.RawMessage(nil), resp.Raw...),
			UsageNotes: resp.UsageNotes,
		}},
		Sections: []record.Section{
			{Heading: "Changeset", Markdown: changesetSection(job.changeset)},
			{Heading: "Selected qualities", Markdown: fmt.Sprintf("rubric: `%s`\n\n%s", job.rubric, selectedSection(job.selected))},
			{Heading: "Classified findings", Markdown: classifiedSection(classified)},
		},
	})
}

func (b *Broker) finishSuccess(job *reviewJob, result CollectReviewResponse) {
	b.mu.Lock()
	job.result = result
	b.mu.Unlock()
	_ = monitor.WriteStatus(job.statusPath, job.status(monitor.StateCompleted, result.LogPath, nil))
	close(job.done)
}

func (b *Broker) finishFailure(job *reviewJob, err error) {
	b.mu.Lock()
	job.err = err
	b.mu.Unlock()
	info := errs.WithTime(errs.InfoFrom(err), time.Now().UTC())
	_ = monitor.WriteStatus(job.statusPath, job.status(monitor.StateFailed, "", info))
	close(job.done)
}

func (b *Broker) CollectReview(ctx context.Context, req CollectReviewRequest) (CollectReviewResponse, error) {
	if err := ctx.Err(); err != nil {
		return CollectReviewResponse{}, err
	}
	if req.ReviewID == "" {
		return CollectReviewResponse{}, errs.New(errs.CodeUserError, "review_id is required", nil, nil)
	}
	key, statusPath, err := b.resolveReview(req.Project, req.ReviewID)
	if err != nil {
		return CollectReviewResponse{}, err
	}

	b.mu.Lock()
	job := b.jobs[key]
	b.mu.Unlock()
	if job != nil {
		select {
		case <-job.done:
		default:
			return CollectReviewResponse{}, errs.New(errs.CodeConflict, "review is still running", nil, map[string]any{"review_id": req.ReviewID, "status_path": job.statusPath})
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if job.err != nil {
			return CollectReviewResponse{}, job.err
		}
		return cloneCollectReviewResponse(job.result), nil
	}

	status, err := monitor.ReadStatus(statusPath)
	if err != nil {
		return CollectReviewResponse{}, errs.New(errs.CodeInternalError, "read review status", err, map[string]any{"status_path": statusPath})
	}
	if status.State == monitor.StateRunning {
		return CollectReviewResponse{}, errs.New(errs.CodeConflict, "review is still running", nil, map[string]any{"review_id": req.ReviewID, "status_path": statusPath})
	}
	if status.State == monitor.StateFailed {
		return CollectReviewResponse{}, errs.New(errs.CodeReviewerFailed, "review failed", errors.New(status.Error.Message), status.Error.Details)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(statusPath), resultFileName))
	if err != nil {
		return CollectReviewResponse{}, errs.New(errs.CodeNotFound, "review result not found", err, nil)
	}
	var stored reviewResultFile
	if err := json.Unmarshal(raw, &stored); err != nil {
		return CollectReviewResponse{}, errs.New(errs.CodeInternalError, "parse review result", err, nil)
	}
	return collectFromStored(stored), nil
}

func (b *Broker) ListReviews(ctx context.Context, project string) (ListReviewsResponse, error) {
	if err := ctx.Err(); err != nil {
		return ListReviewsResponse{}, err
	}
	projects, err := projectDirs(b.options.LogDestination, project)
	if err != nil {
		return ListReviewsResponse{}, errs.New(errs.CodeNotFound, "list review runs", err, nil)
	}
	var out []ReviewSummary
	for _, p := range projects {
		entries, err := os.ReadDir(filepath.Join(b.options.LogDestination, p))
		if err != nil {
			return ListReviewsResponse{}, errs.New(errs.CodeInternalError, "read project review runs", err, map[string]any{"project": p})
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			statusPath := monitor.StatusPath(monitor.ReviewDir(b.options.LogDestination, p, entry.Name()))
			status, err := monitor.ReadStatus(statusPath)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return ListReviewsResponse{}, errs.New(errs.CodeInternalError, "read review status", err, map[string]any{"status_path": statusPath})
			}
			out = append(out, ReviewSummary{
				ReviewID:      status.ReviewID,
				Project:       status.Project,
				Rubric:        status.Rubric,
				State:         status.State,
				ChangesetKind: status.ChangesetKind,
				StartedAt:     status.StartedAt,
				CompletedAt:   status.CompletedAt,
				StatusPath:    status.StatusPath,
				LogPath:       status.LogPath,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt > out[j].StartedAt
	})
	return ListReviewsResponse{Reviews: out}, nil
}

func (b *Broker) resolveReview(project string, reviewID string) (string, string, error) {
	statusPath, err := monitor.FindStatusPath(b.options.LogDestination, project, reviewID)
	if err != nil {
		return "", "", errs.New(errs.CodeNotFound, "review not found", err, map[string]any{"review_id": reviewID})
	}
	status, err := monitor.ReadStatus(statusPath)
	if err != nil {
		return "", "", errs.New(errs.CodeInternalError, "read review status", err, map[string]any{"status_path": statusPath})
	}
	return jobKey(status.Project, status.ReviewID), statusPath, nil
}

func (j *reviewJob) status(state string, logPath string, errInfo *errs.Info) monitor.ReviewStatus {
	status := monitor.ReviewStatus{
		ReviewID:      j.id,
		Project:       j.project,
		Rubric:        j.rubric,
		State:         state,
		ChangesetKind: j.changeset.Kind,
		Reviewer: monitor.ReviewerInfo{
			Name: j.reviewerName,
		},
		StartedAt:  j.startedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		StatusPath: j.statusPath,
		LogPath:    logPath,
		Error:      errInfo,
		Files:      append([]string(nil), j.changeset.Files...),
		Qualities:  monitorQualities(j.selected),
	}
	if state != monitor.StateRunning {
		status.CompletedAt = status.UpdatedAt
	}
	return status
}

func monitorQualities(selected []canon.Selected) []monitor.QualityInfo {
	out := make([]monitor.QualityInfo, 0, len(selected))
	for _, s := range selected {
		out = append(out, monitor.QualityInfo{
			ID:       s.Quality.Head.ID,
			Ref:      s.Quality.Ref,
			Blocking: s.Blocking,
		})
	}
	return out
}

func jobKey(project string, reviewID string) string {
	return project + "/" + reviewID
}

func newReviewID() (string, error) {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (b *Broker) monitorCommand(project string, reviewID string) string {
	var parts []string
	parts = append(parts, "terminus", "monitor")
	if b.options.ConfigPath != "" {
		parts = append(parts, "--config", shellQuote(b.options.ConfigPath))
	}
	parts = append(parts, "--project", shellQuote(project), "--wait", shellQuote(reviewID))
	return strings.Join(parts, " ")
}

func orderTriage(classified []findings.Classified) []TriageFindingOutput {
	blocking := make([]findings.Classified, 0, len(classified))
	advisory := make([]findings.Classified, 0, len(classified))
	for _, c := range classified {
		if c.Blocking {
			blocking = append(blocking, c)
		} else {
			advisory = append(advisory, c)
		}
	}
	ordered := append(blocking, advisory...)
	return classifiedToTriage(ordered)
}

func triageGuidance(findings []TriageFindingOutput) string {
	if len(findings) == 0 {
		return "No findings were returned. Summarize that the review is clean, then decide whether another fresh review is needed."
	}
	return "Present the findings as a concise overview, then walk them one at a time. Blocking findings come first. Do not implement a fix until the user has decided how to handle that finding."
}

func writeResult(path string, result CollectReviewResponse) error {
	file := reviewResultFile{
		ReviewID:     result.ReviewID,
		Project:      result.Project,
		Rubric:       result.Rubric,
		State:        result.State,
		Verdict:      result.Verdict,
		Clean:        result.Clean,
		Summary:      result.Summary,
		LogPath:      result.LogPath,
		PromptPath:   result.PromptPath,
		ReviewerName: result.ReviewerName,
		Raw:          append(json.RawMessage(nil), result.Raw...),
		Findings:     append([]TriageFindingOutput(nil), result.Findings...),
	}
	return writeJSONAtomic(path, file)
}

func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".result-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func projectDirs(logDestination string, project string) ([]string, error) {
	if project != "" {
		return []string{project}, nil
	}
	entries, err := os.ReadDir(logDestination)
	if err != nil {
		return nil, err
	}
	var projects []string
	for _, entry := range entries {
		if entry.IsDir() {
			projects = append(projects, entry.Name())
		}
	}
	sort.Strings(projects)
	return projects, nil
}

func collectFromStored(stored reviewResultFile) CollectReviewResponse {
	resp := CollectReviewResponse{
		ReviewID:     stored.ReviewID,
		Project:      stored.Project,
		Rubric:       stored.Rubric,
		State:        stored.State,
		Verdict:      stored.Verdict,
		Clean:        stored.Clean,
		Summary:      stored.Summary,
		LogPath:      stored.LogPath,
		PromptPath:   stored.PromptPath,
		ReviewerName: stored.ReviewerName,
		Raw:          append(json.RawMessage(nil), stored.Raw...),
		Findings:     append([]TriageFindingOutput(nil), stored.Findings...),
		Guidance:     triageGuidance(stored.Findings),
	}
	if len(resp.Findings) > 0 {
		next := resp.Findings[0]
		resp.NextFinding = &next
	}
	return resp
}

func cloneCollectReviewResponse(in CollectReviewResponse) CollectReviewResponse {
	out := in
	out.Raw = append(json.RawMessage(nil), in.Raw...)
	out.Findings = append([]TriageFindingOutput(nil), in.Findings...)
	if in.NextFinding != nil {
		next := *in.NextFinding
		out.NextFinding = &next
	}
	return out
}

func cloneSelected(in []canon.Selected) []canon.Selected {
	out := make([]canon.Selected, len(in))
	copy(out, in)
	return out
}

func changesetSection(cs changeset.Changeset) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- kind: `%s`\n", cs.Kind))
	b.WriteString(fmt.Sprintf("- files: `%d`\n\n", len(cs.Files)))
	for _, file := range cs.Files {
		b.WriteString(fmt.Sprintf("- `%s`\n", file))
	}
	return b.String()
}

func selectedSection(selected []canon.Selected) string {
	var b strings.Builder
	b.WriteString("| id | ref | blocking |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, s := range selected {
		b.WriteString(fmt.Sprintf("| %s | %s | %t |\n", escapeTable(s.Quality.Head.ID), escapeTable(s.Quality.Ref), s.Blocking))
	}
	return b.String()
}

func classifiedSection(classified []findings.Classified) string {
	raw, err := json.MarshalIndent(classified, "", "  ")
	if err != nil {
		return fmt.Sprintf("failed to render classified findings: %v\n", err)
	}
	var b bytes.Buffer
	b.WriteString("```json\n")
	b.Write(raw)
	b.WriteString("\n```\n")
	return b.String()
}

func escapeTable(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return s
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, " \t\n'\"\\$`") {
		return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
	}
	return s
}
