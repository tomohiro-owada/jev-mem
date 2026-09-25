package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tomohiro-owada/jev-mem/internal/jevmem"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(jevmem.Fail[any]("command_failed", err.Error(), "", nil))
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("command is required")
	}
	switch args[0] {
	case "save":
		return runSave(args[1:], stdin, stdout)
	case "save-decision":
		return runSaveDecision(args[1:], stdin, stdout)
	case "search":
		return runSearch(args[1:], stdin, stdout)
	case "search-analogies":
		return runSearchAnalogies(args[1:], stdin, stdout)
	case "render-report":
		return runRenderReport(args[1:], stdin, stdout)
	case "retry-push":
		return runRetryPush(args[1:], stdout)
	case "sync":
		return runSync(args[1:], stdout)
	case "status":
		return runStatus(args[1:], stdout)
	case "jev-check":
		return runJevCheck(args[1:], stdout)
	case "local-setup":
		return runLocalSetup(args[1:], stdout)
	case "backfill-fingerprints":
		return runBackfillFingerprints(args[1:], stdout)
	case "fingerprint":
		return runFingerprint(args[1:], stdin, stdout)
	case "heatmap":
		return runHeatmap(args[1:], stdin, stdout)
	case "schema":
		return json.NewEncoder(stdout).Encode(schema())
	case "mcp":
		return runMCP(args[1:], stdin, stdout)
	case "http":
		return runHTTP(args[1:])
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func runHeatmap(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	if opts["input"] != "json" {
		return fmt.Errorf("heatmap requires --input json")
	}
	if output := opts["output"]; output != "" && output != "svg" {
		return fmt.Errorf("heatmap only supports --output svg")
	}
	var fingerprint jevmem.Fingerprint
	if err := json.NewDecoder(stdin).Decode(&fingerprint); err != nil {
		return err
	}
	svg, err := jevmem.RenderFingerprintSVG(fingerprint)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, svg)
	return err
}

func newDecisionService(backend string) (*jevmem.Service, func(), error) {
	svc, cleanup, err := newService(true)
	if err != nil {
		return nil, nil, err
	}
	workDir, err := os.Getwd()
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	evaluator, evaluatorCleanup, err := jevmem.NewJevEvaluatorFromEnvironment(workDir, backend)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	svc.WithFingerprintExtractor(&jevmem.JevFingerprintExtractor{Evaluator: evaluator, BatchSize: 25})
	return svc, func() {
		if evaluatorCleanup != nil {
			_ = evaluatorCleanup.Close()
		}
		cleanup()
	}, nil
}

func runFingerprint(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	if opts["input"] != "json" {
		return fmt.Errorf("fingerprint requires --input json")
	}
	var decision jevmem.DecisionInput
	if err := json.NewDecoder(stdin).Decode(&decision); err != nil {
		return err
	}
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}
	evaluator, evaluatorCleanup, err := jevmem.NewJevEvaluatorFromEnvironment(workDir, fingerprintBackendOption(opts))
	if err != nil {
		return err
	}
	if evaluatorCleanup != nil {
		defer evaluatorCleanup.Close()
	}
	extractorBatchSize := 25
	if local, ok := evaluator.(*jevmem.LocalLayaEvaluator); ok {
		extractorBatchSize = 100
		local.BatchSize = atoiDefault(opts["laya-batch-size"], 64)
	}
	extractor := &jevmem.JevFingerprintExtractor{Evaluator: evaluator, BatchSize: extractorBatchSize}
	fingerprint, err := extractor.Extract(context.Background(), decision)
	if err != nil {
		return err
	}
	return writeResponse(stdout, opts["output"], jevmem.OK(fingerprint))
}

func runJevCheck(args []string, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}
	evaluator, evaluatorCleanup, err := jevmem.NewJevEvaluatorFromEnvironment(workDir, fingerprintBackendOption(opts))
	if err != nil {
		return err
	}
	if evaluatorCleanup != nil {
		defer evaluatorCleanup.Close()
	}
	response, err := evaluator.Evaluate(context.Background(), map[string]any{
		"application": "jev-mem",
		"purpose":     "API connectivity check",
	}, map[string]jevmem.JevQuestion{
		"connection_check": {
			Type:         "noul",
			Instructions: "Is this state explicitly describing a connectivity check for the jev-mem application?",
			Criteria: map[string]string{
				"true":  "The state explicitly says it is a connectivity check for jev-mem.",
				"false": "The state describes something else.",
			},
		},
	})
	if err != nil {
		return err
	}
	answer, ok := response.Answers["connection_check"]
	if !ok || answer.Type != "noul" || answer.Noul == nil {
		return fmt.Errorf("Jev returned an invalid connectivity-check answer")
	}
	return json.NewEncoder(stdout).Encode(map[string]any{
		"ok":    true,
		"model": response.Model,
		"usage": response.Usage,
	})
}

func runLocalSetup(args []string, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}
	result, err := jevmem.SetupLocalLaya(context.Background(), workDir, os.Stderr)
	if err != nil {
		return err
	}
	return writeResponse(stdout, opts["output"], jevmem.OK(result))
}

func runBackfillFingerprints(args []string, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	repoDir := strings.TrimSpace(opts["repo"])
	remoteURL := strings.TrimSpace(opts["remote"])
	if repoDir == "" {
		cfg, err := jevmem.LoadConfig("")
		if err != nil {
			return err
		}
		repoDir, remoteURL = cfg.GitDir, cfg.RemoteURL
	}
	if repoDir == "" {
		return fmt.Errorf("--repo is required when git_dir is not configured")
	}
	repoDir, err = filepath.Abs(repoDir)
	if err != nil {
		return err
	}
	repo := jevmem.GitRepo{Dir: repoDir, RemoteURL: remoteURL}
	if err := repo.Ensure(context.Background()); err != nil {
		return err
	}
	dirty, err := repo.Dirty(context.Background())
	if err != nil {
		return err
	}
	if dirty && opts["resume"] != "true" {
		return fmt.Errorf("repository has uncommitted changes; inspect them and rerun with --resume only if they are an interrupted fingerprint backfill")
	}
	if !dirty {
		if err := repo.PullRebase(context.Background()); err != nil {
			return err
		}
	}
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}
	evaluator, evaluatorCleanup, err := jevmem.NewJevEvaluatorFromEnvironment(workDir, fingerprintBackendOption(opts))
	if err != nil {
		return err
	}
	if evaluatorCleanup != nil {
		defer evaluatorCleanup.Close()
	}
	extractorBatchSize := 25
	if local, ok := evaluator.(*jevmem.LocalLayaEvaluator); ok {
		extractorBatchSize = 100
		local.BatchSize = atoiDefault(opts["laya-batch-size"], 64)
	}
	extractor := &jevmem.JevFingerprintExtractor{Evaluator: evaluator, BatchSize: extractorBatchSize}
	result, err := jevmem.BackfillLegacyFingerprints(context.Background(), extractor, jevmem.BackfillOptions{
		RepoDir: repoDir,
		Limit:   atoiDefault(opts["limit"], 0),
		DryRun:  opts["dry-run"] == "true",
	}, func(progress jevmem.BackfillProgress) {
		percent := float64(0)
		if progress.Total > 0 {
			percent = 100 * float64(progress.Processed) / float64(progress.Total)
		}
		fmt.Fprintf(os.Stderr, "[%d/%d %5.1f%%] updated=%d skipped=%d failed=%d elapsed=%s eta=%s file=%s\n",
			progress.Processed, progress.Total, percent, progress.Updated, progress.Skipped, progress.Failed,
			shortDuration(progress.Elapsed), shortDuration(progress.ETA), progress.Path)
		if progress.Error != "" {
			fmt.Fprintf(os.Stderr, "  error: %s\n", progress.Error)
		}
	})
	if err != nil {
		return err
	}
	if !result.DryRun && result.Updated > 0 && opts["commit"] == "true" {
		commit, err := repo.AddCommit(context.Background(), ".", fmt.Sprintf("Add decision fingerprints to %d memories", result.Updated))
		if err != nil {
			return err
		}
		result.CommitHash = commit
		if opts["push"] == "true" {
			if err := repo.Push(context.Background()); err != nil {
				return fmt.Errorf("fingerprints were committed locally as %s but push failed: %w", commit, err)
			}
			result.Pushed = true
		}
	}
	return writeResponse(stdout, opts["output"], jevmem.OK(result))
}

func shortDuration(value time.Duration) string {
	if value < time.Second {
		return value.Round(100 * time.Millisecond).String()
	}
	return value.Round(time.Second).String()
}

func newService(ensureAssets bool) (*jevmem.Service, func(), error) {
	cfg, err := jevmem.LoadConfig("")
	if err != nil {
		return nil, nil, err
	}
	if ensureAssets {
		if err := jevmem.EnsureAssets(context.Background(), cfg); err != nil {
			return nil, nil, err
		}
	}
	idx, err := jevmem.OpenIndex(cfg.IndexPath)
	if err != nil {
		return nil, nil, err
	}
	emb := &jevmem.E5Embedder{Config: cfg}
	cleanup := func() {
		_ = idx.Close()
		if err := emb.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "embedder close failed:", err)
		}
	}
	return jevmem.NewService(cfg, idx, emb), cleanup, nil
}

func runSave(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	var req jevmem.SaveRequest
	if opts["input"] == "json" {
		if err := json.NewDecoder(stdin).Decode(&req); err != nil {
			return err
		}
	} else {
		if opts["content"] != "" && opts["file"] != "" {
			return fmt.Errorf("--content and --file cannot be used together")
		}
		body := opts["content"]
		if opts["file"] != "" {
			b, err := os.ReadFile(opts["file"])
			if err != nil {
				return err
			}
			body = string(b)
		}
		if body == "" {
			return fmt.Errorf("content is required")
		}
		workspace := opts["workspace"]
		if workspace == "" {
			wd, _ := os.Getwd()
			workspace = wd
		}
		req = jevmem.SaveRequest{CurrentWorkspacePath: workspace, Title: opts["title"], Content: body, DryRun: opts["dry-run"] == "true"}
	}
	svc, cleanup, err := newService(true)
	if err != nil {
		return err
	}
	defer cleanup()
	return writeResponse(stdout, opts["output"], svc.Save(context.Background(), req))
}

func runSaveDecision(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	if opts["input"] != "json" {
		return fmt.Errorf("save-decision requires --input json")
	}
	var req jevmem.SaveDecisionRequest
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		return err
	}
	svc, cleanup, err := newDecisionService(fingerprintBackendOption(opts))
	if err != nil {
		return err
	}
	defer cleanup()
	return writeResponse(stdout, opts["output"], svc.SaveDecision(context.Background(), req))
}

func runSearch(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, rest, err := parseArgs(args)
	if err != nil {
		return err
	}
	var req jevmem.SearchRequest
	if opts["input"] == "json" {
		if err := json.NewDecoder(stdin).Decode(&req); err != nil {
			return err
		}
	} else {
		if len(rest) < 1 {
			return fmt.Errorf("query is required")
		}
		workspace := opts["workspace"]
		all := opts["all"] == "true"
		if workspace == "" && !all {
			wd, _ := os.Getwd()
			workspace = wd
		}
		req = jevmem.SearchRequest{Query: rest[0], CurrentWorkspacePath: workspace, All: all, Limit: atoiDefault(opts["limit"], 10), SnippetChars: atoiDefault(opts["snippet-chars"], 0)}
		if opts["fields"] != "" {
			req.Fields = strings.Split(opts["fields"], ",")
		}
	}
	svc, cleanup, err := newService(true)
	if err != nil {
		return err
	}
	defer cleanup()
	return writeSearchResponse(stdout, opts["output"], svc.Search(context.Background(), req))
}

func runSearchAnalogies(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	if opts["input"] != "json" {
		return fmt.Errorf("search-analogies requires --input json")
	}
	var req jevmem.AnalogSearchRequest
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		return err
	}
	svc, cleanup, err := newDecisionService(fingerprintBackendOption(opts))
	if err != nil {
		return err
	}
	defer cleanup()
	response := svc.SearchAnalogies(context.Background(), req)
	if reportPath := strings.TrimSpace(opts["html-report"]); reportPath != "" && response.OK {
		html, err := jevmem.RenderAnalogSearchHTML(response.Data, time.Now())
		if err != nil {
			return err
		}
		reportPath, err = filepath.Abs(reportPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(reportPath, []byte(html), 0o644); err != nil {
			return err
		}
	}
	return writeResponse(stdout, opts["output"], response)
}

func runRenderReport(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, rest, err := parseArgs(args)
	if err != nil {
		return err
	}
	if len(rest) != 0 || opts["input"] != "json" {
		return fmt.Errorf("render-report requires --input json and no positional arguments")
	}
	if output := opts["output"]; output != "" && output != "html" {
		return fmt.Errorf("render-report only supports --output html")
	}
	var response jevmem.Response[jevmem.AnalogSearchData]
	if err := json.NewDecoder(stdin).Decode(&response); err != nil {
		return err
	}
	if !response.OK {
		return fmt.Errorf("cannot render an unsuccessful search response")
	}
	html, err := jevmem.RenderAnalogSearchHTML(response.Data, time.Now())
	if err != nil {
		return err
	}
	if reportPath := strings.TrimSpace(opts["file"]); reportPath != "" {
		reportPath, err = filepath.Abs(reportPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(reportPath, []byte(html), 0o644)
	}
	_, err = io.WriteString(stdout, html)
	return err
}

func runRetryPush(args []string, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	svc, cleanup, err := newService(false)
	if err != nil {
		return err
	}
	defer cleanup()
	return writeResponse(stdout, opts["output"], svc.RetryPush(context.Background(), jevmem.RetryPushRequest{DryRun: opts["dry-run"] == "true"}))
}

func runSync(args []string, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	svc, cleanup, err := newService(true)
	if err != nil {
		return err
	}
	defer cleanup()
	return writeResponse(stdout, opts["output"], svc.Sync(context.Background()))
}

func runStatus(args []string, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	svc, cleanup, err := newService(false)
	if err != nil {
		return err
	}
	defer cleanup()
	return writeResponse(stdout, opts["output"], svc.Status(context.Background()))
}

func runMCP(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, _, err := parseArgs(args)
	if err != nil {
		return err
	}
	if backend := fingerprintBackendOption(opts); backend != "" {
		resolved, err := jevmem.ResolveJevBackend("", backend)
		if err != nil {
			return err
		}
		// Decision MCP tools run in child processes; the environment is the
		// explicit, inherited backend selection for those children.
		if err := os.Setenv("JEV_BACKEND", resolved); err != nil {
			return err
		}
	}
	// The MCP transport is a long-running loop. The embedding model (~470MB of
	// native ONNX memory) is expensive to load, and once loaded the OS does not
	// reclaim it in-process even after the session is destroyed (the C allocator
	// keeps the pages). To keep an idle MCP process small — instead of N
	// concurrent sessions each pinning ~600MB — the embedding-heavy tools
	// (save/search) are delegated to a short-lived child process that loads the
	// model, does the work, and exits, at which point the OS reclaims all of it.
	//
	// The parent service here never loads the model; it only serves retry_push
	// (git-only, no assets), so newService(false) keeps startup asset-free and
	// offline-safe.
	svc, cleanup, err := newService(false)
	if err != nil {
		return err
	}
	defer cleanup()
	scanner := bufio.NewScanner(stdin)
	writer := bufio.NewWriter(stdout)
	defer writer.Flush()
	for scanner.Scan() {
		line := scanner.Bytes()
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		resp := handleRPC(svc, req)
		b, _ := json.Marshal(resp)
		_, _ = writer.Write(b)
		_ = writer.WriteByte('\n')
		_ = writer.Flush()
	}
	return scanner.Err()
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Result  any            `json:"result,omitempty"`
	Error   map[string]any `json:"error,omitempty"`
}

func handleRPC(svc *jevmem.Service, req rpcRequest) rpcResponse {
	switch req.Method {
	case "initialize":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"protocolVersion": negotiateProtocolVersion(req.Params), "serverInfo": map[string]any{"name": "jev-mem", "version": "0.1.0"}, "capabilities": map[string]any{"tools": map[string]any{}}}}
	case "tools/list":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": mcpTools()}}
	case "tools/call":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: callTool(svc, req.Params)}
	default:
		if req.ID == nil {
			return rpcResponse{}
		}
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: map[string]any{"code": -32601, "message": "method not found"}}
	}
}

// supportedProtocolVersions lists the MCP revisions this server speaks, newest
// first. Streamable HTTP was introduced in 2025-03-26, so the stdio-era
// 2024-11-05 alone is not enough once the HTTP transport is in play.
var supportedProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// negotiateProtocolVersion echoes the client's requested revision when this
// server supports it, and otherwise answers with the newest one it speaks.
func negotiateProtocolVersion(raw json.RawMessage) string {
	var in struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(raw, &in); err == nil {
		for _, v := range supportedProtocolVersions {
			if in.ProtocolVersion == v {
				return v
			}
		}
	}
	return supportedProtocolVersions[0]
}

func callTool(svc *jevmem.Service, raw json.RawMessage) any {
	var in struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return toolText(jevmem.Fail[any]("invalid_request", err.Error(), "", nil))
	}
	switch in.Name {
	case "save_memory":
		// Delegate to a short-lived child so the ~470MB model is reclaimed by
		// the OS the moment the child exits (see runMCP for why).
		return runToolViaSubprocess("save", in.Arguments)
	case "search_memory":
		return runToolViaSubprocess("search", in.Arguments)
	case "save_decision":
		return runToolViaSubprocess("save-decision", in.Arguments)
	case "search_analogies":
		return runToolViaSubprocess("search-analogies", in.Arguments)
	case "retry_push":
		var req jevmem.RetryPushRequest
		_ = json.Unmarshal(in.Arguments, &req)
		return toolText(svc.RetryPush(context.Background(), req))
	default:
		return toolText(jevmem.Fail[any]("unknown_tool", "unknown tool", "", nil))
	}
}

// runToolViaSubprocess executes an embedding-heavy tool (save/search) in a
// fresh child process using the CLI subcommand of the same binary, forwarding
// the MCP arguments as the child's JSON stdin and relaying its JSON response.
// Running in a child guarantees the native model memory is returned to the OS
// when the child exits, keeping the resident MCP server small while idle.
func runToolViaSubprocess(sub string, args json.RawMessage) any {
	exe, err := os.Executable()
	if err != nil {
		return toolText(jevmem.Fail[any]("server_error", err.Error(), "", nil))
	}
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	cmd := exec.Command(exe, sub, "--input", "json", "--output", "json")
	cmd.Stdin = bytes.NewReader(args)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		msg := "embedding subprocess produced no output"
		if runErr != nil {
			msg = runErr.Error()
		}
		return toolText(jevmem.Fail[any]("server_error", msg, "", nil))
	}
	// The child already emits the canonical Response JSON; relay it verbatim as
	// the tool's text content.
	return map[string]any{"content": []map[string]any{{"type": "text", "text": out}}}
}

func toolText(v any) map[string]any {
	b, _ := json.Marshal(v)
	return map[string]any{"content": []map[string]any{{"type": "text", "text": string(b)}}}
}

func mcpTools() []map[string]any {
	return []map[string]any{
		{"name": "save_memory", "description": "Save a memory", "inputSchema": schema()["tools"].(map[string]any)["save_memory"]},
		{"name": "search_memory", "description": "Search memories", "inputSchema": schema()["tools"].(map[string]any)["search_memory"]},
		{"name": "save_decision", "description": "Save a decision with a Jev-generated structural fingerprint", "inputSchema": schema()["tools"].(map[string]any)["save_decision"]},
		{"name": "search_analogies", "description": "Search decisions using semantic and structural fingerprint similarity", "inputSchema": schema()["tools"].(map[string]any)["search_analogies"]},
		{"name": "retry_push", "description": "Retry pushing local commits", "inputSchema": schema()["tools"].(map[string]any)["retry_push"]},
	}
}

func schema() map[string]any {
	decisionProperties := map[string]any{
		"decision": map[string]any{"type": "string"}, "rationale": map[string]any{"type": "string"}, "evidence": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "context": map[string]any{"type": "string"}, "decision_time": map[string]any{"type": "string"}, "focal_option": map[string]any{"type": "string"}, "reference_option": map[string]any{"type": "string"}, "chosen_response": map[string]any{"type": "string"}, "evaluation_horizon": map[string]any{"type": "string"},
	}
	decisionSchema := map[string]any{"type": "object", "required": []string{"decision", "rationale", "focal_option", "chosen_response"}, "properties": decisionProperties}
	return map[string]any{
		"tools": map[string]any{
			"save_memory":      map[string]any{"type": "object", "required": []string{"current_workspace_path", "title", "content"}, "properties": map[string]any{"current_workspace_path": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}, "dry_run": map[string]any{"type": "boolean"}}},
			"search_memory":    map[string]any{"type": "object", "required": []string{"query"}, "properties": map[string]any{"query": map[string]any{"type": "string"}, "current_workspace_path": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}, "all": map[string]any{"type": "boolean"}, "fields": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "snippet_chars": map[string]any{"type": "integer"}}},
			"save_decision":    map[string]any{"type": "object", "required": []string{"current_workspace_path", "title", "decision"}, "properties": map[string]any{"current_workspace_path": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "decision": decisionSchema, "dry_run": map[string]any{"type": "boolean"}}},
			"search_analogies": map[string]any{"type": "object", "required": []string{"decision"}, "properties": map[string]any{"decision": decisionSchema, "current_workspace_path": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}, "all": map[string]any{"type": "boolean"}, "semantic_weight": map[string]any{"type": "number", "minimum": 0}, "fingerprint_weight": map[string]any{"type": "number", "minimum": 0}}},
			"retry_push":       map[string]any{"type": "object", "properties": map[string]any{"dry_run": map[string]any{"type": "boolean"}}},
		},
		"commands": map[string]any{
			"save":                  map[string]any{"output": []string{"json", "text"}},
			"search":                map[string]any{"output": []string{"json", "ndjson", "text"}},
			"save-decision":         map[string]any{"input": []string{"json"}, "output": []string{"json", "text"}},
			"search-analogies":      map[string]any{"input": []string{"json"}, "output": []string{"json", "text"}, "options": []string{"html-report"}},
			"render-report":         map[string]any{"input": []string{"json"}, "output": []string{"html"}, "options": []string{"file"}},
			"sync":                  map[string]any{"output": []string{"json", "text"}},
			"status":                map[string]any{"output": []string{"json", "text"}},
			"retry-push":            map[string]any{"output": []string{"json", "text"}},
			"jev-check":             map[string]any{"output": []string{"json"}},
			"local-setup":           map[string]any{"output": []string{"json"}},
			"backfill-fingerprints": map[string]any{"output": []string{"json", "text"}, "options": []string{"repo", "remote", "local", "limit", "laya-batch-size", "dry-run", "resume", "commit", "push"}},
			"fingerprint": map[string]any{
				"input":  []string{"json"},
				"output": []string{"json", "text"},
			},
			"heatmap": map[string]any{"input": []string{"json"}, "output": []string{"svg"}},
			"schema":  map[string]any{"output": []string{"json"}},
			"mcp":     map[string]any{"transport": "stdio"},
		},
	}
}

func writeResponse(stdout io.Writer, output string, v any) error {
	if output == "" || output == "json" {
		return json.NewEncoder(stdout).Encode(v)
	}
	if output != "text" {
		return fmt.Errorf("unsupported output: %s", output)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	_, err := fmt.Fprintln(stdout, string(b))
	return err
}

func writeSearchResponse(stdout io.Writer, output string, resp jevmem.Response[jevmem.SearchData]) error {
	if output == "ndjson" {
		if !resp.OK {
			return json.NewEncoder(stdout).Encode(resp)
		}
		for _, result := range resp.Data.Results {
			if err := json.NewEncoder(stdout).Encode(result); err != nil {
				return err
			}
		}
		return nil
	}
	return writeResponse(stdout, output, resp)
}

func parseArgs(args []string) (map[string]string, []string, error) {
	opts := map[string]string{}
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			rest = append(rest, arg)
			continue
		}
		key := strings.TrimPrefix(arg, "--")
		if key == "" {
			return nil, nil, fmt.Errorf("invalid option")
		}
		if strings.Contains(key, "=") {
			k, v, _ := strings.Cut(key, "=")
			opts[k] = v
			continue
		}
		if key == "all" || key == "dry-run" || key == "non-interactive" || key == "local" || key == "resume" || key == "commit" || key == "push" {
			opts[key] = "true"
			continue
		}
		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("--%s requires a value", key)
		}
		opts[key] = args[i+1]
		i++
	}
	return opts, rest, nil
}

func fingerprintBackendOption(opts map[string]string) string {
	if opts["local"] == "true" {
		return jevmem.LocalLayaBackend
	}
	if value := opts["jev-backend"]; value != "" {
		return value
	}
	return opts["jev"]
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
