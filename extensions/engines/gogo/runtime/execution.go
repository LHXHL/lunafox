package gogoruntime

import (
	"context"
	"errors"
	"fmt"
	"os"

	enginecontract "github.com/yyhuni/lunafox/engines/gogo/contract"
)

// Runtime owns the gogo-specific lifecycle around the shared Engine API
// ports. Agent terminal reporting remains the sole owner of Task state.
type Runtime struct{}

func New() *Runtime {
	return &Runtime{}
}

func (runtime *Runtime) Execute(ctx context.Context, execution *enginecontract.Execution) error {
	if runtime == nil {
		return errors.New("Gogo runtime is required")
	}
	if ctx == nil {
		return errors.New("execution context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if execution == nil {
		return errors.New("execution is required")
	}
	if execution.Progress == nil {
		return errors.New("Gogo progress port is required")
	}
	if execution.Results.HostPorts == nil {
		return errors.New("typed HostPort result port is required")
	}
	if err := validateGogoConfig(execution.Config.Gogo); err != nil {
		return err
	}
	if _, err := requireGogoWorkspace(execution.Workspace); err != nil {
		return err
	}

	subdomainsPath := ""
	if execution.Input.Subdomains != nil {
		path, err := execution.Input.Subdomains.Path(ctx)
		if err != nil {
			return fmt.Errorf("materialize Subdomains input: %w", err)
		}
		subdomainsPath = path
	}

	candidatesPath, candidateCount, resolvedDomains, err := prepareGogoCandidates(ctx, execution.Target, subdomainsPath, execution.Workspace)
	if err != nil {
		return fmt.Errorf("prepare gogo candidates: %w", err)
	}
	if candidateCount == 0 {
		if err := execution.Progress.Report(ctx, "input_ready candidates=0"); err != nil {
			return gogoProgressError(ctx, "input_ready", err)
		}
		return nil
	}
	if err := execution.Progress.Report(ctx, fmt.Sprintf(
		"input_ready candidates=%d resolvedDomains=%d", candidateCount, resolvedDomains,
	)); err != nil {
		return gogoProgressError(ctx, "input_ready", err)
	}

	artifactPath, err := gogoArtifactPath(execution.Workspace)
	if err != nil {
		return err
	}
	args, err := BuildGogoArgs(execution.Config.Gogo, candidatesPath, artifactPath)
	if err != nil {
		return err
	}
	if err := execution.Progress.Report(ctx, fmt.Sprintf("scan_started candidates=%d", candidateCount)); err != nil {
		return gogoProgressError(ctx, "scan_started", err)
	}
	if err := runGogoProcess(ctx, GogoBinary, args); err != nil {
		return fmt.Errorf("run gogo: %w", err)
	}

	artifact, err := os.Open(artifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := execution.Progress.Report(ctx, "scan_completed records=0"); err != nil {
				return gogoProgressError(ctx, "scan_completed", err)
			}
			return nil
		}
		return fmt.Errorf("open gogo artifact: %w", err)
	}
	defer func() { _ = artifact.Close() }()

	summary, err := parseGogoOutput(ctx, artifact, execution.Results)
	if err != nil {
		return fmt.Errorf("parse gogo output: %w", err)
	}
	if err := execution.Progress.Report(ctx, fmt.Sprintf(
		"scan_completed records=%d hostPorts=%d websites=%d technologies=%d vulnerabilities=%d",
		summary.Records, summary.HostPorts, summary.Websites, summary.Technologies, summary.Vulnerabilities,
	)); err != nil {
		return gogoProgressError(ctx, "scan_completed", err)
	}
	return nil
}
