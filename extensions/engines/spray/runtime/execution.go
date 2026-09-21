package sprayruntime

import (
	"context"
	"errors"
	"fmt"
	"os"

	enginecontract "github.com/yyhuni/lunafox/engines/spray/contract"
)

// Runtime owns the spray-specific lifecycle around the shared Engine API
// ports. Agent terminal reporting remains the sole owner of Task state.
type Runtime struct{}

func New() *Runtime {
	return &Runtime{}
}

func (runtime *Runtime) Execute(ctx context.Context, execution *enginecontract.Execution) error {
	if runtime == nil {
		return errors.New("Spray runtime is required")
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
		return errors.New("Spray progress port is required")
	}
	if execution.Results.Directories == nil {
		return errors.New("typed Directory result port is required")
	}
	if err := validateSprayConfig(execution.Config.Spray); err != nil {
		return err
	}
	if _, err := requireSprayWorkspace(execution.Workspace); err != nil {
		return err
	}
	if execution.Input.WebsiteURLs == nil {
		return errors.New("typed WebsiteURLs input handle is required")
	}
	websiteURLsPath, err := execution.Input.WebsiteURLs.Path(ctx)
	if err != nil {
		return fmt.Errorf("materialize WebsiteURLs input: %w", err)
	}

	candidatesPath, candidateCount, err := prepareSprayCandidates(ctx, websiteURLsPath, execution.Workspace)
	if err != nil {
		return fmt.Errorf("prepare spray candidates: %w", err)
	}
	if candidateCount == 0 {
		if err := execution.Progress.Report(ctx, "input_ready websiteCandidates=0"); err != nil {
			return sprayProgressError(ctx, "input_ready", err)
		}
		return nil
	}
	if err := execution.Progress.Report(ctx, fmt.Sprintf("input_ready websiteCandidates=%d", candidateCount)); err != nil {
		return sprayProgressError(ctx, "input_ready", err)
	}

	artifactPath, err := sprayArtifactPath(execution.Workspace)
	if err != nil {
		return err
	}
	args, err := BuildSprayArgs(execution.Config.Spray, candidatesPath, artifactPath)
	if err != nil {
		return err
	}
	if err := execution.Progress.Report(ctx, fmt.Sprintf("scan_started websiteCandidates=%d", candidateCount)); err != nil {
		return sprayProgressError(ctx, "scan_started", err)
	}
	if err := runSprayProcess(ctx, SprayBinary, args, execution.Workspace); err != nil {
		return fmt.Errorf("run spray: %w", err)
	}

	artifact, err := os.Open(artifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := execution.Progress.Report(ctx, "scan_completed records=0"); err != nil {
				return sprayProgressError(ctx, "scan_completed", err)
			}
			return nil
		}
		return fmt.Errorf("open spray artifact: %w", err)
	}
	defer func() { _ = artifact.Close() }()

	summary, err := parseSprayOutput(ctx, artifact, execution.Results)
	if err != nil {
		return fmt.Errorf("parse spray output: %w", err)
	}
	if err := execution.Progress.Report(ctx, fmt.Sprintf(
		"scan_completed records=%d directories=%d technologies=%d",
		summary.Records, summary.Directories, summary.Technologies,
	)); err != nil {
		return sprayProgressError(ctx, "scan_completed", err)
	}
	return nil
}
