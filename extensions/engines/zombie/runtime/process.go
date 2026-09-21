package zombieruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type zombieProcess interface {
	StdoutPipe() (io.ReadCloser, error)
	Start() error
	Wait() error
	Kill() error
}

type zombieProcessFactory func(context.Context, string, []string, io.Writer) zombieProcess

type execZombieProcess struct {
	command *exec.Cmd
}

func (process *execZombieProcess) StdoutPipe() (io.ReadCloser, error) {
	return process.command.StdoutPipe()
}

func (process *execZombieProcess) Start() error { return process.command.Start() }

func (process *execZombieProcess) Wait() error { return process.command.Wait() }

func (process *execZombieProcess) Kill() error {
	if process.command.Process == nil {
		return nil
	}
	return process.command.Process.Kill()
}

func newExecZombieProcess(ctx context.Context, binary string, args []string, stderr io.Writer) zombieProcess {
	command := exec.CommandContext(ctx, binary, args...)
	command.Stderr = stderr
	return &execZombieProcess{command: command}
}

// runZombieProcess owns the single zombie child. stdout is drained; results
// are read from the JSONL artifact afterwards so a killed child cannot erase
// complete records written before termination.
func runZombieProcess(ctx context.Context, binary string, args []string) error {
	if ctx == nil {
		return errors.New("zombie process context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if binary == "" {
		return errors.New("zombie binary is required")
	}
	process := newExecZombieProcess(ctx, binary, args, os.Stderr)
	if process == nil {
		return errors.New("create zombie process")
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open zombie stdout: %w", err)
	}
	if err := process.Start(); err != nil {
		closeErr := stdout.Close()
		startErr := fmt.Errorf("start zombie: %w", err)
		if closeErr != nil {
			return errors.Join(startErr, fmt.Errorf("close zombie stdout: %w", closeErr))
		}
		return startErr
	}
	if _, err := io.Copy(io.Discard, stdout); err != nil {
		_ = process.Kill()
		_ = process.Wait()
		return fmt.Errorf("drain zombie stdout: %w", err)
	}
	if err := process.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("wait for zombie: %w", err)
	}
	return nil
}

func zombieArtifactPath(workspace string) (string, error) {
	workspacePath, err := requireZombieWorkspace(workspace)
	if err != nil {
		return "", err
	}
	return filepath.Join(workspacePath, "zombie.jsonl"), nil
}
