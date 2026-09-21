package zombieruntime

import (
	"context"
	"errors"
	"fmt"
	"os"

	enginecontract "github.com/yyhuni/lunafox/engines/zombie/contract"
)

// Runtime owns the zombie-specific lifecycle around the shared Engine API
// ports. Agent terminal reporting remains the sole owner of Task state.
type Runtime struct{}

func New() *Runtime {
	return &Runtime{}
}

func (runtime *Runtime) Execute(ctx context.Context, execution *enginecontract.Execution) error {
	if runtime == nil {
		return errors.New("Zombie runtime is required")
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
		return errors.New("Zombie progress port is required")
	}
	if execution.Results.Vulnerabilities == nil {
		return errors.New("typed Vulnerability result port is required")
	}
	if err := validateZombieConfig(execution.Config.Zombie); err != nil {
		return err
	}
	if _, err := requireZombieWorkspace(execution.Workspace); err != nil {
		return err
	}
	if execution.Input.HostPorts == nil {
		return errors.New("typed HostPorts input handle is required")
	}
	hostPortsPath, err := execution.Input.HostPorts.Path(ctx)
	if err != nil {
		return fmt.Errorf("materialize HostPorts input: %w", err)
	}

	plan, err := prepareZombieTargets(ctx, hostPortsPath, execution.Workspace, execution.Config.Zombie.Services)
	if err != nil {
		return fmt.Errorf("prepare zombie targets: %w", err)
	}
	if plan.Targets == 0 {
		if err := execution.Progress.Report(ctx, fmt.Sprintf(
			"input_ready targets=0 skippedPorts=%d", plan.SkippedPorts,
		)); err != nil {
			return zombieProgressError(ctx, "input_ready", err)
		}
		return nil
	}
	if err := execution.Progress.Report(ctx, fmt.Sprintf(
		"input_ready targets=%d skippedPorts=%d services=%s",
		plan.Targets, plan.SkippedPorts, formatServiceCounts(plan.ServiceCounts),
	)); err != nil {
		return zombieProgressError(ctx, "input_ready", err)
	}

	artifactPath, err := zombieArtifactPath(execution.Workspace)
	if err != nil {
		return err
	}
	args, err := BuildZombieArgs(execution.Config.Zombie, plan.Path, artifactPath)
	if err != nil {
		return err
	}
	if err := execution.Progress.Report(ctx, fmt.Sprintf("scan_started targets=%d", plan.Targets)); err != nil {
		return zombieProgressError(ctx, "scan_started", err)
	}
	if err := runZombieProcess(ctx, ZombieBinary, args); err != nil {
		return fmt.Errorf("run zombie: %w", err)
	}

	artifact, err := os.Open(artifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := execution.Progress.Report(ctx, "scan_completed records=0"); err != nil {
				return zombieProgressError(ctx, "scan_completed", err)
			}
			return nil
		}
		return fmt.Errorf("open zombie artifact: %w", err)
	}
	defer func() { _ = artifact.Close() }()

	summary, err := parseZombieOutput(ctx, artifact, execution.Results)
	if err != nil {
		return fmt.Errorf("parse zombie output: %w", err)
	}
	if err := execution.Progress.Report(ctx, fmt.Sprintf(
		"scan_completed records=%d credentials=%d", summary.Records, summary.Vulnerabilities,
	)); err != nil {
		return zombieProgressError(ctx, "scan_completed", err)
	}
	return nil
}
