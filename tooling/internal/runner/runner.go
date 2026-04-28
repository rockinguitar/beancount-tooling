package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
)

type Engine string

const (
	EngineDocker Engine = "docker"
	EngineLocal  Engine = "local"
)

type Request struct {
	Engine            Engine
	FinanceDir        string
	BeancountFilename string
	Query             string
}

func RunQuery(ctx context.Context, request Request) ([]byte, error) {
	switch request.Engine {
	case "", EngineDocker:
		return runDockerQuery(ctx, request)
	case EngineLocal:
		return runLocalQuery(ctx, request)
	default:
		return nil, fmt.Errorf("unsupported engine %q", request.Engine)
	}
}

func runDockerQuery(ctx context.Context, request Request) ([]byte, error) {
	ledgerPath := path.Join("/ledger", request.BeancountFilename)

	cmd := exec.CommandContext(
		ctx,
		"docker-compose",
		"--profile",
		"tools",
		"run",
		"--rm",
		"-T",
		"--no-deps",
		"beancount-tools",
		"bean-query",
		"-f",
		"csv",
		ledgerPath,
		request.Query,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run bean-query via docker: %w\n%s", err, stderr.String())
	}

	return output, nil
}

func runLocalQuery(ctx context.Context, request Request) ([]byte, error) {
	ledgerPath := filepath.Join(request.FinanceDir, filepath.FromSlash(request.BeancountFilename))
	cmd := exec.CommandContext(ctx, "bean-query", "-f", "csv", ledgerPath, request.Query)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run bean-query locally: %w\n%s", err, stderr.String())
	}

	return output, nil
}
