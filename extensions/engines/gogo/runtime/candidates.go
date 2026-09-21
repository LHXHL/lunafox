package gogoruntime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	enginecontract "github.com/yyhuni/lunafox/engines/gogo/contract"
)

// prepareGogoCandidates materializes the gogo IP/CIDR list input in the
// writable task workspace. gogo only accepts IP/CIDR targets, so domain
// values (the Target baseline for domain scans and the Subdomains fact
// lines) are resolved to IPv4 addresses first. CIDR and IP values pass
// through verbatim for gogo's own expansion.
func prepareGogoCandidates(ctx context.Context, target enginecontract.Target, subdomainsPath, workDir string) (path string, count uint64, resolvedDomains uint64, err error) {
	if ctx == nil {
		return "", 0, 0, errors.New("candidate context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", 0, 0, err
	}
	workspacePath, err := requireGogoWorkspace(workDir)
	if err != nil {
		return "", 0, 0, err
	}

	path = filepath.Join(workspacePath, "gogo-candidates.txt")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, 0, fmt.Errorf("create gogo candidate file: %w", err)
	}
	cleanup := func(cause error) (string, uint64, uint64, error) {
		closeErr := file.Close()
		removeErr := os.Remove(path)
		if closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("close gogo candidate file: %w", closeErr))
		}
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			cause = errors.Join(cause, fmt.Errorf("remove incomplete gogo candidate file: %w", removeErr))
		}
		return "", 0, 0, cause
	}
	writer := bufio.NewWriterSize(file, 64*1024)
	seen := make(map[string]struct{})
	writeCandidate := func(value string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if value == "" {
			return nil
		}
		if _, duplicate := seen[value]; duplicate {
			return nil
		}
		seen[value] = struct{}{}
		if _, err := writer.WriteString(value + "\n"); err != nil {
			return fmt.Errorf("write gogo candidate: %w", err)
		}
		count++
		return nil
	}

	emitTarget := func(value string) error {
		if ip := net.ParseIP(value); ip != nil {
			return writeCandidate(value)
		}
		if _, _, cidrErr := net.ParseCIDR(value); cidrErr == nil {
			return writeCandidate(value)
		}
		ips, resolveErr := resolveIPv4(ctx, value)
		if resolveErr != nil {
			return fmt.Errorf("resolve Target %q: %w", value, resolveErr)
		}
		resolvedDomains++
		for _, ip := range ips {
			if err := writeCandidate(ip); err != nil {
				return err
			}
		}
		return nil
	}

	switch target.Type {
	case enginecontract.TargetTypeIP, enginecontract.TargetTypeCIDR:
		if err := emitTarget(target.Value); err != nil {
			return cleanup(err)
		}
	case enginecontract.TargetTypeDomain:
		if err := emitTarget(target.Value); err != nil {
			return cleanup(err)
		}
	default:
		return cleanup(fmt.Errorf("unsupported Target type %q", target.Type))
	}

	if subdomainsPath != "" {
		source, err := os.Open(subdomainsPath)
		if err != nil {
			return cleanup(fmt.Errorf("open Subdomains facts: %w", err))
		}
		scanner := bufio.NewScanner(source)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				_ = source.Close()
				return cleanup(err)
			}
			line := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			ips, err := resolveIPv4(ctx, line)
			if err != nil {
				_ = source.Close()
				return cleanup(fmt.Errorf("resolve Subdomains fact %q: %w", line, err))
			}
			resolvedDomains++
			for _, ip := range ips {
				if err := writeCandidate(ip); err != nil {
					_ = source.Close()
					return cleanup(err)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			_ = source.Close()
			return cleanup(fmt.Errorf("read Subdomains facts: %w", err))
		}
		if err := source.Close(); err != nil {
			return cleanup(fmt.Errorf("close Subdomains facts: %w", err))
		}
	}

	if err := writer.Flush(); err != nil {
		return cleanup(fmt.Errorf("flush gogo candidates: %w", err))
	}
	if err := file.Close(); err != nil {
		return "", 0, 0, fmt.Errorf("close gogo candidate file: %w", err)
	}
	return path, count, resolvedDomains, nil
}

func resolveIPv4(ctx context.Context, domain string) ([]string, error) {
	var resolver net.Resolver
	ips, err := resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return nil, err
	}
	var addresses []string
	for _, ip := range ips {
		if ipv4 := ip.IP.To4(); ipv4 != nil {
			addresses = append(addresses, ipv4.String())
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no IPv4 address found")
	}
	return addresses, nil
}

func requireGogoWorkspace(workspace string) (string, error) {
	if workspace == "" {
		return "", errors.New("Gogo workspace is required")
	}
	if workspace != strings.TrimSpace(workspace) {
		return "", errors.New("Gogo workspace must not have surrounding whitespace")
	}
	if !filepath.IsAbs(workspace) {
		return "", errors.New("Gogo workspace must be absolute")
	}
	if filepath.Clean(workspace) != workspace {
		return "", errors.New("Gogo workspace must be clean")
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return "", fmt.Errorf("inspect Gogo workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("Gogo workspace must be an existing directory")
	}
	return workspace, nil
}

// validateGogoConfig enforces the engine.json constraints before the tool runs.
func validateGogoConfig(config enginecontract.GogoConfig) error {
	if !config.Enabled {
		return errors.New("Gogo section must be enabled")
	}
	if config.PortTag == "" && config.Ports == "" {
		return errors.New("Gogo port-tag or ports is required")
	}
	if config.Concurrency < 1 || config.Concurrency > 4000 {
		return errors.New("Gogo concurrency must be between 1 and 4000")
	}
	if config.Timeout < 1 || config.Timeout > 30 {
		return errors.New("Gogo timeout must be between 1 and 30")
	}
	switch config.Mod {
	case "default", "s", "ss", "sc":
	default:
		return errors.New("Gogo mod must be one of default, s, ss, sc")
	}
	return nil
}
