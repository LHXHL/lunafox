package sprayruntime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	enginecontract "github.com/yyhuni/lunafox/engines/spray/contract"
)

// prepareSprayCandidates materializes the spray URL-list input in the writable
// task workspace. The mounted WebsiteURLs fact file stays read-only; lines are
// copied verbatim after canonical http(s) URL validation and exact dedup.
func prepareSprayCandidates(ctx context.Context, websiteURLsPath, workDir string) (path string, count uint64, err error) {
	if ctx == nil {
		return "", 0, errors.New("candidate context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	if websiteURLsPath == "" {
		return "", 0, errors.New("WebsiteURLs facts path is required")
	}
	workspacePath, err := requireSprayWorkspace(workDir)
	if err != nil {
		return "", 0, err
	}
	source, err := os.Open(websiteURLsPath)
	if err != nil {
		return "", 0, fmt.Errorf("open WebsiteURLs facts: %w", err)
	}
	defer func() { _ = source.Close() }()

	path = filepath.Join(workspacePath, "spray-candidates.txt")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, fmt.Errorf("create spray candidate file: %w", err)
	}
	cleanup := func(cause error) (string, uint64, error) {
		closeErr := file.Close()
		removeErr := os.Remove(path)
		if closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("close spray candidate file: %w", closeErr))
		}
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			cause = errors.Join(cause, fmt.Errorf("remove incomplete spray candidate file: %w", removeErr))
		}
		return "", 0, cause
	}

	writer := bufio.NewWriterSize(file, 64*1024)
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return cleanup(err)
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := validateCandidateURL(line); err != nil {
			return cleanup(fmt.Errorf("WebsiteURLs candidate %q: %w", line, err))
		}
		if _, duplicate := seen[line]; duplicate {
			continue
		}
		seen[line] = struct{}{}
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return cleanup(fmt.Errorf("write spray candidate: %w", err))
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return cleanup(fmt.Errorf("read WebsiteURLs facts: %w", err))
	}
	if err := writer.Flush(); err != nil {
		return cleanup(fmt.Errorf("flush spray candidates: %w", err))
	}
	if err := file.Close(); err != nil {
		return "", 0, fmt.Errorf("close spray candidate file: %w", err)
	}
	return path, count, nil
}

func validateCandidateURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Host == "" {
		return errors.New("URL authority is required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("URL scheme must be http or https")
	}
	return nil
}

func requireSprayWorkspace(workspace string) (string, error) {
	if workspace == "" {
		return "", errors.New("Spray workspace is required")
	}
	if workspace != strings.TrimSpace(workspace) {
		return "", errors.New("Spray workspace must not have surrounding whitespace")
	}
	if !filepath.IsAbs(workspace) {
		return "", errors.New("Spray workspace must be absolute")
	}
	if filepath.Clean(workspace) != workspace {
		return "", errors.New("Spray workspace must be clean")
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return "", fmt.Errorf("inspect Spray workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("Spray workspace must be an existing directory")
	}
	return workspace, nil
}

// validateSprayConfig enforces the engine.json constraints before the tool runs.
func validateSprayConfig(config enginecontract.SprayConfig) error {
	if !config.Enabled {
		return errors.New("Spray section must be enabled")
	}
	if config.Wordlist == "" {
		return errors.New("Spray wordlist resource is required")
	}
	if info, err := os.Stat(config.Wordlist); err != nil || info.IsDir() {
		return fmt.Errorf("Spray wordlist resource %q is not readable", config.Wordlist)
	}
	if config.Pool < 1 || config.Pool > 50 {
		return errors.New("Spray pool must be between 1 and 50")
	}
	if config.Threads < 1 || config.Threads > 100 {
		return errors.New("Spray threads must be between 1 and 100")
	}
	if config.RequestTimeout < 1 || config.RequestTimeout > 120 {
		return errors.New("Spray request-timeout must be between 1 and 120")
	}
	if config.Mod != "path" && config.Mod != "host" {
		return errors.New("Spray mod must be path or host")
	}
	return nil
}
