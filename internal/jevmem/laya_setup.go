package jevmem

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const DefaultLocalLayaPackage = "laya-mlx==0.2.0"

type LocalLayaSetupResult struct {
	Python   string `json:"python"`
	Model    string `json:"model"`
	Revision string `json:"revision,omitempty"`
	Ready    bool   `json:"ready"`
}

// SetupLocalLaya creates an isolated Python environment, installs laya-mlx,
// and downloads the configured checkpoint. It never modifies the user's
// global Python environment.
func SetupLocalLaya(ctx context.Context, workDir string, log io.Writer) (LocalLayaSetupResult, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return LocalLayaSetupResult{}, fmt.Errorf("local Laya requires Apple Silicon on macOS (darwin/arm64)")
	}
	values, err := environmentValues(workDir)
	if err != nil {
		return LocalLayaSetupResult{}, err
	}
	model := firstValue(values, "JEV_LOCAL_MODEL", "LAYA_MODEL")
	if model == "" {
		model = DefaultLocalLayaModel
	}
	revision := firstValue(values, "JEV_LOCAL_MODEL_REVISION", "LAYA_MODEL_REVISION")
	if revision == "" && model == DefaultLocalLayaModel {
		revision = DefaultLocalLayaRevision
	}
	root := filepath.Join(defaultDataDir(), "laya-mlx")
	venv := filepath.Join(root, ".venv")
	venvPython := filepath.Join(venv, "bin", "python")
	if runtime.GOOS == "windows" {
		venvPython = filepath.Join(venv, "Scripts", "python.exe")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return LocalLayaSetupResult{}, err
	}
	bootstrap, err := localSetupPython(values)
	if err != nil {
		return LocalLayaSetupResult{}, err
	}
	if _, err := os.Stat(venvPython); err != nil {
		if err := runSetupCommand(ctx, log, bootstrap, "-m", "venv", venv); err != nil {
			return LocalLayaSetupResult{}, fmt.Errorf("create local Laya virtual environment: %w", err)
		}
	}
	if err := runSetupCommand(ctx, log, venvPython, "-m", "pip", "install", "--upgrade", DefaultLocalLayaPackage); err != nil {
		return LocalLayaSetupResult{}, fmt.Errorf("install laya-mlx: %w", err)
	}
	preload := "import sys; import laya_mlx as laya; laya.load(sys.argv[1], revision=(sys.argv[2] or None), device='gpu', dtype='float16'); print('local Laya model ready', file=sys.stderr)"
	if err := runSetupCommand(ctx, log, venvPython, "-c", preload, model, revision); err != nil {
		return LocalLayaSetupResult{}, fmt.Errorf("download and load local Laya model: %w", err)
	}
	return LocalLayaSetupResult{Python: venvPython, Model: model, Revision: revision, Ready: true}, nil
}

func localSetupPython(values map[string]string) (string, error) {
	if configured := firstValue(values, "JEV_LOCAL_BOOTSTRAP_PYTHON"); configured != "" {
		return configured, nil
	}
	for _, candidate := range []string{"python3.12", "python3.11"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errorsNewPythonRequirement()
}

func errorsNewPythonRequirement() error {
	return fmt.Errorf("Python 3.11 or 3.12 is required for local Laya; install it or set JEV_LOCAL_BOOTSTRAP_PYTHON")
}

func runSetupCommand(ctx context.Context, log io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if log != nil {
		cmd.Stdout = log
		cmd.Stderr = log
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
