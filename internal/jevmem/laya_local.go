package jevmem

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	LocalLayaBackend         = "local"
	CloudJevBackend          = "cloud"
	DefaultLocalLayaModel    = "aac6fef/laya-multilingual-mlx"
	DefaultLocalLayaRevision = "f2b4faf51023039425946074e2cf1361d2db11d5"
)

//go:embed laya_bridge.py
var layaBridgeScript string

type LocalLayaEvaluator struct {
	Python    string
	Model     string
	Revision  string
	Device    string
	DType     string
	BatchSize int

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	stderr  bytes.Buffer
}

type layaBridgeReply struct {
	Ready    bool        `json:"ready,omitempty"`
	Model    string      `json:"model,omitempty"`
	OK       bool        `json:"ok,omitempty"`
	Response JevResponse `json:"response,omitempty"`
	Error    string      `json:"error,omitempty"`
}

func NewLocalLayaEvaluatorFromEnvironment(workDir string) (*LocalLayaEvaluator, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return nil, fmt.Errorf("local Laya requires Apple Silicon on macOS (darwin/arm64)")
	}
	values, err := environmentValues(workDir)
	if err != nil {
		return nil, err
	}
	python := firstValue(values, "JEV_LOCAL_PYTHON", "LAYA_PYTHON")
	if python == "" {
		python = defaultLocalLayaPython()
	}
	model := firstValue(values, "JEV_LOCAL_MODEL", "LAYA_MODEL")
	if model == "" {
		model = DefaultLocalLayaModel
	}
	revision := firstValue(values, "JEV_LOCAL_MODEL_REVISION", "LAYA_MODEL_REVISION")
	if revision == "" && model == DefaultLocalLayaModel {
		revision = DefaultLocalLayaRevision
	}
	device := firstValue(values, "JEV_LOCAL_DEVICE", "LAYA_DEVICE")
	if device == "" {
		device = "gpu"
	}
	dtype := firstValue(values, "JEV_LOCAL_DTYPE", "LAYA_DTYPE")
	if dtype == "" {
		dtype = "float16"
	}
	batchSize := 16
	if raw := firstValue(values, "JEV_LOCAL_BATCH_SIZE", "LAYA_BATCH_SIZE"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 1 {
			return nil, fmt.Errorf("JEV_LOCAL_BATCH_SIZE must be a positive integer")
		}
		batchSize = parsed
	}
	return &LocalLayaEvaluator{Python: python, Model: model, Revision: revision, Device: device, DType: dtype, BatchSize: batchSize}, nil
}

func (e *LocalLayaEvaluator) Evaluate(ctx context.Context, state any, questions map[string]JevQuestion) (JevResponse, error) {
	if e == nil {
		return JevResponse{}, errors.New("local Laya evaluator is required")
	}
	if len(questions) == 0 {
		return JevResponse{}, errors.New("at least one Laya question is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.start(ctx); err != nil {
		return JevResponse{}, err
	}
	localQuestions, denseAttributes := layaQuestions(questions)
	payload, err := json.Marshal(map[string]any{"state": state, "questions": localQuestions})
	if err != nil {
		return JevResponse{}, err
	}
	if _, err := e.stdin.Write(append(payload, '\n')); err != nil {
		e.stopLocked()
		return JevResponse{}, fmt.Errorf("write to local Laya worker: %w", err)
	}
	type scanResult struct {
		line []byte
		err  error
	}
	result := make(chan scanResult, 1)
	go func() {
		if e.scanner.Scan() {
			result <- scanResult{line: append([]byte(nil), e.scanner.Bytes()...)}
			return
		}
		result <- scanResult{err: e.scanner.Err()}
	}()
	select {
	case <-ctx.Done():
		e.stopLocked()
		return JevResponse{}, ctx.Err()
	case scanned := <-result:
		if scanned.err != nil || len(scanned.line) == 0 {
			message := strings.TrimSpace(e.stderr.String())
			e.stopLocked()
			if message == "" {
				message = "worker exited without a response"
			}
			return JevResponse{}, fmt.Errorf("local Laya worker failed: %s", message)
		}
		var reply layaBridgeReply
		if err := json.Unmarshal(scanned.line, &reply); err != nil {
			return JevResponse{}, fmt.Errorf("decode local Laya response: %w", err)
		}
		if !reply.OK {
			return JevResponse{}, fmt.Errorf("local Laya evaluation failed: %s", reply.Error)
		}
		wrapDenseLayaAnswers(&reply.Response, denseAttributes)
		return reply.Response, nil
	}
}

// Laya is a dense typed-score model. Its multilingual checkpoint does not
// implement Jev's evidence-grounded applicable/unknown/not_applicable
// semantics reliably, so the adapter sends only score questions and wraps a
// successful score as an applicable Jev answer. The score entropy confidence
// is preserved on both answers, allowing jev-mem to down-weight uncertain
// local fingerprints without changing its canonical data model.
func layaQuestions(questions map[string]JevQuestion) (map[string]JevQuestion, []string) {
	local := make(map[string]JevQuestion, len(questions))
	dense := make([]string, 0, len(questions)/2)
	for id, question := range questions {
		if strings.HasSuffix(id, ".applicability") {
			base := strings.TrimSuffix(id, ".applicability")
			if score, ok := questions[base+".score"]; ok && score.Type == "score" {
				dense = append(dense, base)
				continue
			}
		}
		local[id] = question
	}
	return local, dense
}

func wrapDenseLayaAnswers(response *JevResponse, attributes []string) {
	if response == nil || response.Answers == nil {
		return
	}
	for _, attribute := range attributes {
		score, ok := response.Answers[attribute+".score"]
		if !ok || score.Type != "score" || score.Score == nil {
			continue
		}
		confidence := answerConfidence(score)
		response.Answers[attribute+".applicability"] = JevAnswer{
			Type:       "choice",
			Choice:     string(Applicable),
			Confidence: &confidence,
			Probabilities: map[string]float64{
				string(Applicable): 1,
			},
		}
	}
}

func (e *LocalLayaEvaluator) start(ctx context.Context) error {
	if e.cmd != nil {
		return nil
	}
	python := strings.TrimSpace(e.Python)
	if python == "" {
		python = defaultLocalLayaPython()
	}
	model := strings.TrimSpace(e.Model)
	if model == "" {
		model = DefaultLocalLayaModel
	}
	device := strings.TrimSpace(e.Device)
	if device == "" {
		device = "gpu"
	}
	dtype := strings.TrimSpace(e.DType)
	if dtype == "" {
		dtype = "float16"
	}
	batchSize := e.BatchSize
	if batchSize < 1 {
		batchSize = 16
	}
	args := []string{"-u", "-c", layaBridgeScript, "--model", model, "--device", device, "--dtype", dtype, "--batch-size", strconv.Itoa(batchSize)}
	if revision := strings.TrimSpace(e.Revision); revision != "" {
		args = append(args, "--revision", revision)
	}
	cmd := exec.Command(python, args...)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	e.stderr.Reset()
	cmd.Stderr = &e.stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start local Laya worker with %s: %w", python, err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	e.cmd, e.stdin, e.scanner = cmd, stdin, scanner

	ready := make(chan scanResult, 1)
	go func() {
		if scanner.Scan() {
			ready <- scanResult{line: append([]byte(nil), scanner.Bytes()...)}
			return
		}
		ready <- scanResult{err: scanner.Err()}
	}()
	select {
	case <-ctx.Done():
		e.stopLocked()
		return ctx.Err()
	case result := <-ready:
		if result.err != nil || len(result.line) == 0 {
			message := strings.TrimSpace(e.stderr.String())
			e.stopLocked()
			if message == "" {
				message = "worker exited during startup"
			}
			return fmt.Errorf("local Laya startup failed: %s; run `jev-mem local-setup` first", message)
		}
		var reply layaBridgeReply
		if err := json.Unmarshal(result.line, &reply); err != nil || !reply.Ready {
			e.stopLocked()
			return fmt.Errorf("local Laya worker returned an invalid startup response")
		}
		return nil
	}
}

type scanResult struct {
	line []byte
	err  error
}

func (e *LocalLayaEvaluator) Close() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopLocked()
}

func (e *LocalLayaEvaluator) stopLocked() error {
	if e.cmd == nil {
		return nil
	}
	_ = e.stdin.Close()
	done := make(chan error, 1)
	go func(cmd *exec.Cmd) { done <- cmd.Wait() }(e.cmd)
	var err error
	select {
	case err = <-done:
	case <-time.After(2 * time.Second):
		_ = e.cmd.Process.Kill()
		err = <-done
	}
	e.cmd, e.stdin, e.scanner = nil, nil, nil
	if err != nil && !strings.Contains(err.Error(), "signal: killed") {
		return err
	}
	return nil
}

func ResolveJevBackend(workDir, override string) (string, error) {
	backend := strings.ToLower(strings.TrimSpace(override))
	if backend == "" {
		values, err := environmentValues(workDir)
		if err != nil {
			return "", err
		}
		backend = strings.ToLower(firstValue(values, "JEV_BACKEND", "FINGERPRINT_BACKEND"))
	}
	if backend == "" || backend == "jev" || backend == "api" {
		return CloudJevBackend, nil
	}
	if backend == "laya" || backend == "laya-mlx" {
		return LocalLayaBackend, nil
	}
	if backend != CloudJevBackend && backend != LocalLayaBackend {
		return "", fmt.Errorf("unsupported Jev backend %q: expected cloud or local", backend)
	}
	return backend, nil
}

func NewJevEvaluatorFromEnvironment(workDir, override string) (JevEvaluator, io.Closer, error) {
	backend, err := ResolveJevBackend(workDir, override)
	if err != nil {
		return nil, nil, err
	}
	if backend == LocalLayaBackend {
		evaluator, err := NewLocalLayaEvaluatorFromEnvironment(workDir)
		return evaluator, evaluator, err
	}
	client, err := NewJevClientFromEnvironment(workDir)
	return client, nil, err
}

func environmentValues(workDir string) (map[string]string, error) {
	values := map[string]string{}
	if workDir == "" {
		return values, nil
	}
	fileValues, err := readEnvFile(filepath.Join(workDir, ".env.local"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for key, value := range fileValues {
		values[key] = value
	}
	return values, nil
}

func firstValue(fileValues map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
		if value := strings.TrimSpace(fileValues[name]); value != "" {
			return value
		}
	}
	return ""
}

func defaultLocalLayaPython() string {
	venvPython := filepath.Join(defaultDataDir(), "laya-mlx", ".venv", "bin", "python")
	if runtime.GOOS == "windows" {
		venvPython = filepath.Join(defaultDataDir(), "laya-mlx", ".venv", "Scripts", "python.exe")
	}
	if info, err := os.Stat(venvPython); err == nil && !info.IsDir() {
		return venvPython
	}
	for _, candidate := range []string{"python3.12", "python3.11", "python3"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return "python3"
}
