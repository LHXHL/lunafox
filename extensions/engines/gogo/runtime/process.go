package gogoruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type gogoProcess interface {
	StdoutPipe() (io.ReadCloser, error)
	Start() error
	Wait() error
	Kill() error
}

type gogoProcessFactory func(context.Context, string, []string, io.Writer) gogoProcess

type execGogoProcess struct {
	command *exec.Cmd
}

func (process *execGogoProcess) StdoutPipe() (io.ReadCloser, error) {
	return process.command.StdoutPipe()
}

func (process *execGogoProcess) Start() error { return process.command.Start() }

func (process *execGogoProcess) Wait() error { return process.command.Wait() }

func (process *execGogoProcess) Kill() error {
	if process.command.Process == nil {
		return nil
	}
	return process.command.Process.Kill()
}

func newExecGogoProcess(ctx context.Context, binary string, args []string, stderr io.Writer) gogoProcess {
	command := exec.CommandContext(ctx, binary, args...)
	command.Stderr = stderr
	return &execGogoProcess{command: command}
}

// runGogoProcess owns the single gogo child. stdout is drained (results are
// read from the JSONL artifact afterwards so a killed child cannot erase
// complete records written before termination).
func runGogoProcess(ctx context.Context, binary string, args []string) error {
	if ctx == nil {
		return errors.New("gogo process context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if binary == "" {
		return errors.New("gogo binary is required")
	}
	process := newExecGogoProcess(ctx, binary, args, os.Stderr)
	if process == nil {
		return errors.New("create gogo process")
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open gogo stdout: %w", err)
	}
	if err := process.Start(); err != nil {
		closeErr := stdout.Close()
		startErr := fmt.Errorf("start gogo: %w", err)
		if closeErr != nil {
			return errors.Join(startErr, fmt.Errorf("close gogo stdout: %w", closeErr))
		}
		return startErr
	}
	if _, err := io.Copy(io.Discard, stdout); err != nil {
		_ = process.Kill()
		_ = process.Wait()
		return fmt.Errorf("drain gogo stdout: %w", err)
	}
	if err := process.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("wait for gogo: %w", err)
	}
	return nil
}

func gogoArtifactPath(workspace string) (string, error) {
	workspacePath, err := requireGogoWorkspace(workspace)
	if err != nil {
		return "", err
	}
	return filepath.Join(workspacePath, "gogo.jsonl"), nil
}
