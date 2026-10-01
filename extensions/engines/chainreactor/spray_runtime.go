package chainreactor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yyhuni/lunafox/contracts/results"
)

// SprayConfig is the Engine-owned, bounded subset of spray's path fuzzing
// controls. A generated Engine Facade must project manifest values into it.
type SprayConfig struct {
	Pool           int
	ThreadsPerPool int
	RatePerPool    int
	RequestTimeout time.Duration
	Timeout        time.Duration
}

func (config SprayConfig) validate() error {
	if config.Pool < 1 || config.Pool > 20 || config.ThreadsPerPool < 1 || config.ThreadsPerPool > 100 || config.Pool*config.ThreadsPerPool > 500 {
		return errors.New("spray pool and thread limits exceeded")
	}
	if config.RatePerPool < 1 || config.RatePerPool > 1000 {
		return errors.New("spray rate limit must be between 1 and 1000 per pool")
	}
	if config.RequestTimeout < time.Second || config.RequestTimeout > 120*time.Second || config.RequestTimeout%time.Second != 0 {
		return errors.New("spray request timeout must be whole seconds between 1 and 120")
	}
	if config.Timeout < time.Minute || config.Timeout > 24*time.Hour || config.Timeout%time.Second != 0 {
		return errors.New("spray timeout must be whole seconds between 60 and 86400")
	}
	return nil
}

// RunSpray runs one bounded, path-only spray process. websiteURLsPath and
// wordlistPath are already materialized by the Engine Facade. A zero-record
// website input succeeds without starting the tool.
func RunSpray(ctx context.Context, binary, websiteURLsPath, wordlistPath, workspace string, config SprayConfig, visit func(results.Directory) error) (ParseSummary, error) {
	if ctx == nil || visit == nil || binary == "" || workspace == "" {
		return ParseSummary{}, errors.New("spray context, tool, workspace, and result visitor are required")
	}
	if err := config.validate(); err != nil {
		return ParseSummary{}, err
	}
	if err := requireRegularFile(wordlistPath); err != nil {
		return ParseSummary{}, fmt.Errorf("spray wordlist: %w", err)
	}
	origins, baselines, err := websiteInputOrigins(ctx, websiteURLsPath)
	if err != nil {
		return ParseSummary{}, err
	}
	if len(origins) == 0 {
		return ParseSummary{}, nil
	}
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		return ParseSummary{}, errors.New("spray workspace must be an existing directory")
	}
	workDir, err := os.MkdirTemp(workspace, "spray-")
	if err != nil {
		return ParseSummary{}, fmt.Errorf("create spray work directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	outputPath := filepath.Join(workDir, "results.json")
	args := []string{
		"-l", websiteURLsPath,
		"-d", wordlistPath,
		"-m", "path",
		"-O", "json",
		"-f", outputPath,
		"--no-bar", "--no-stat", "-q",
		"--rate-limit", strconv.Itoa(config.RatePerPool),
		"-P", strconv.Itoa(config.Pool),
		"-t", strconv.Itoa(config.ThreadsPerPool),
		"-T", strconv.Itoa(int(config.RequestTimeout / time.Second)),
		"--deadline", strconv.Itoa(int(config.Timeout / time.Second)),
	}
	runCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, args...)
	command.Dir = workDir
	if err := command.Run(); err != nil {
		if runErr := runCtx.Err(); runErr != nil {
			return ParseSummary{}, fmt.Errorf("spray execution: %w", runErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ParseSummary{}, fmt.Errorf("spray exited with status %d", exitErr.ExitCode())
		}
		return ParseSummary{}, fmt.Errorf("start spray: %w", err)
	}
	file, err := os.Open(outputPath)
	if errors.Is(err, os.ErrNotExist) {
		return ParseSummary{}, nil
	}
	if err != nil {
		return ParseSummary{}, fmt.Errorf("open spray result: %w", err)
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return ParseSummary{}, errors.New("spray result must be a regular file")
	}
	return ParseSprayJSONL(ctx, file, func(item *url.URL) bool {
		_, ok := origins[originKey(item)]
		_, baseline := baselines[baselineKey(item)]
		return ok && !baseline
	}, visit)
}

func requireRegularFile(path string) error {
	if path == "" {
		return errors.New("path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("path must name a regular file")
	}
	return nil
}

func websiteInputOrigins(ctx context.Context, path string) (map[string]struct{}, map[string]struct{}, error) {
	if err := requireRegularFile(path); err != nil {
		return nil, nil, fmt.Errorf("website URLs input: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 4096)
	origins := make(map[string]struct{})
	baselines := make(map[string]struct{})
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		value := strings.TrimSpace(scanner.Text())
		if value == "" {
			continue
		}
		if _, err := results.ValidateObservedAssetURL(value); err != nil {
			return nil, nil, fmt.Errorf("invalid website URL input: %w", err)
		}
		parsed, err := url.Parse(value)
		if err != nil {
			return nil, nil, fmt.Errorf("parse website URL input: %w", err)
		}
		origins[originKey(parsed)] = struct{}{}
		baselines[baselineKey(parsed)] = struct{}{}
		if len(origins) > 100000 {
			return nil, nil, errors.New("spray website origin limit exceeded")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read website URLs input: %w", err)
	}
	return origins, baselines, nil
}

func originKey(value *url.URL) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(value.Scheme) + "://" + strings.ToLower(value.Host)
}

func baselineKey(value *url.URL) string {
	if value == nil {
		return ""
	}
	return originKey(value) + strings.TrimSuffix(value.EscapedPath(), "/")
}
