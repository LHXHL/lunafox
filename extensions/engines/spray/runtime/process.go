package sprayruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type sprayProcess interface {
	StdoutPipe() (io.ReadCloser, error)
	Start() error
	Wait() error
	Kill() error
}

type sprayProcessFactory func(context.Context, string, []string, io.Writer) sprayProcess

type execSprayProcess struct {
	command *exec.Cmd
}

func (process *execSprayProcess) StdoutPipe() (io.ReadCloser, error) {
	return process.command.StdoutPipe()
}

func (process *execSprayProcess) Start() error { return process.command.Start() }

func (process *execSprayProcess) Wait() error { return process.command.Wait() }

func (process *execSprayProcess) Kill() error {
	if process.command.Process == nil {
		return nil
	}
	return process.command.Process.Kill()
}

func newExecSprayProcess(ctx context.Context, binary string, args []string, stderr io.Writer) sprayProcess {
	command := exec.CommandContext(ctx, binary, args...)
	command.Stderr = stderr
	return &execSprayProcess{command: command}
}

// runSprayProcess owns the single spray child. stdout (progress noise under
// --quiet) is drained; results are read from the JSONL artifact afterwards.
func runSprayProcess(ctx context.Context, binary string, args []string, workspace string) error {
	if ctx == nil {
		return errors.New("spray process context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if binary == "" {
		return errors.New("spray binary is required")
	}
	process := newExecSprayProcess(ctx, binary, args, os.Stderr)
	if process == nil {
		return errors.New("create spray process")
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open spray stdout: %w", err)
	}
	if err := process.Start(); err != nil {
		closeErr := stdout.Close()
		startErr := fmt.Errorf("start spray: %w", err)
		if closeErr != nil {
			return errors.Join(startErr, fmt.Errorf("close spray stdout: %w", closeErr))
		}
		return startErr
	}
	if _, err := io.Copy(io.Discard, stdout); err != nil {
		_ = process.Kill()
		_ = process.Wait()
		return fmt.Errorf("drain spray stdout: %w", err)
	}
	if err := process.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("wait for spray: %w", err)
	}
	return nil
}

func sprayArtifactPath(workspace string) (string, error) {
	workspacePath, err := requireSprayWorkspace(workspace)
	if err != nil {
		return "", err
	}
	return filepath.Join(workspacePath, "spray.jsonl"), nil
}
