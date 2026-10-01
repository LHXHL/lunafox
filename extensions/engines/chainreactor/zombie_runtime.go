package chainreactor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ZombieConfig bounds one explicit credential assessment. The current zombie
// CLI has no request-per-second control; this is for isolated validation until
// a paced execution path and protected credential resource are integrated.
type ZombieConfig struct {
	Timeout        time.Duration
	RequestTimeout time.Duration
	MaxAttempts    int
	CheckAnonymous bool
}

func (config ZombieConfig) validate() error {
	if config.Timeout < time.Minute || config.Timeout > time.Hour || config.Timeout%time.Second != 0 {
		return errors.New("zombie timeout must be whole seconds between 60 and 3600")
	}
	if config.RequestTimeout < time.Second || config.RequestTimeout > 30*time.Second || config.RequestTimeout%time.Second != 0 {
		return errors.New("zombie request timeout must be whole seconds between 1 and 30")
	}
	if config.MaxAttempts < 1 || config.MaxAttempts > 1000 {
		return errors.New("zombie maximum attempts must be between 1 and 1000")
	}
	return nil
}

// RunZombieLocalVerification invokes official zombie with private credential
// files and passes only redacted successful findings to visit. Production use
// requires a protected Agent materialization path and an AuthFinding result
// contract; ordinary wordlist resources must not supply passwordFile.
func RunZombieLocalVerification(ctx context.Context, binary string, endpoints []ServiceEndpoint, usernameFile, passwordFile, workspace string, config ZombieConfig, visit func(AuthFinding) error) (ParseSummary, error) {
	if ctx == nil || binary == "" || workspace == "" || visit == nil {
		return ParseSummary{}, errors.New("zombie context, tool, workspace, and finding visitor are required")
	}
	if err := config.validate(); err != nil {
		return ParseSummary{}, err
	}
	if len(endpoints) == 0 {
		return ParseSummary{}, nil
	}
	if len(endpoints) > 1000 {
		return ParseSummary{}, errors.New("zombie endpoint limit exceeded")
	}
	users, err := countCredentialLines(usernameFile, 10, 128)
	if err != nil {
		return ParseSummary{}, fmt.Errorf("zombie usernames: %w", err)
	}
	passwords, err := countCredentialLines(passwordFile, 10, 1024)
	if err != nil {
		return ParseSummary{}, fmt.Errorf("zombie passwords: %w", err)
	}
	if len(endpoints)*users*passwords > config.MaxAttempts {
		return ParseSummary{}, errors.New("zombie credential attempt budget exceeded")
	}
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		return ParseSummary{}, errors.New("zombie workspace must be an existing directory")
	}
	workDir, err := os.MkdirTemp(workspace, "zombie-")
	if err != nil {
		return ParseSummary{}, fmt.Errorf("create zombie work directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	targetPath := filepath.Join(workDir, "targets.txt")
	targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return ParseSummary{}, fmt.Errorf("create zombie target file: %w", err)
	}
	allowed := make(map[ServiceEndpoint]struct{}, len(endpoints))
	lines := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		line, err := ZombieTargetLine(endpoint)
		if err != nil {
			_ = targetFile.Close()
			return ParseSummary{}, err
		}
		if _, exists := allowed[endpoint]; exists {
			continue
		}
		allowed[endpoint] = struct{}{}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	for _, line := range lines {
		if _, err := targetFile.WriteString(line + "\n"); err != nil {
			_ = targetFile.Close()
			return ParseSummary{}, fmt.Errorf("write zombie target file: %w", err)
		}
	}
	if err := targetFile.Close(); err != nil {
		return ParseSummary{}, fmt.Errorf("close zombie target file: %w", err)
	}
	outputPath := filepath.Join(workDir, "results.json")
	args := []string{
		"-I", targetPath,
		"-U", usernameFile,
		"-P", passwordFile,
		"-f", outputPath,
		"-O", "json",
		"-t", "1", "--concurrency", "1",
		"--timeout", strconv.Itoa(int(config.RequestTimeout / time.Second)),
		"--no-honeypot", "-q",
	}
	if !config.CheckAnonymous {
		args = append(args, "--no-unauth")
	}
	runCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, args...)
	command.Dir = workDir
	if err := command.Run(); err != nil {
		if runErr := runCtx.Err(); runErr != nil {
			return ParseSummary{}, fmt.Errorf("zombie execution: %w", runErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ParseSummary{}, fmt.Errorf("zombie exited with status %d", exitErr.ExitCode())
		}
		return ParseSummary{}, fmt.Errorf("start zombie: %w", err)
	}
	file, err := os.Open(outputPath)
	if errors.Is(err, os.ErrNotExist) {
		return ParseSummary{}, nil
	}
	if err != nil {
		return ParseSummary{}, fmt.Errorf("open zombie result: %w", err)
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return ParseSummary{}, errors.New("zombie result must be a regular file")
	}
	return ParseZombieJSONL(ctx, file, func(endpoint ServiceEndpoint) bool {
		_, ok := allowed[endpoint]
		return ok
	}, visit)
}

func countCredentialLines(path string, maximum, maximumBytes int) (int, error) {
	if err := requireRegularFile(path); err != nil {
		return 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, maximumBytes+1), maximumBytes+1)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.Contains(line, "\r") || len(line) > maximumBytes {
			return 0, errors.New("credential file contains an invalid line")
		}
		count++
		if count > maximum {
			return 0, errors.New("credential file exceeds line limit")
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, errors.New("credential file has an oversized or unreadable line")
	}
	if count == 0 {
		return 0, errors.New("credential file is empty")
	}
	return count, nil
}
