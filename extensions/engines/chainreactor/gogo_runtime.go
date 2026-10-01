package chainreactor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yyhuni/lunafox/contracts/results"
)

const maximumGogoCandidates = 65536

type GogoTarget struct {
	Type  string
	Value string
}

type GogoConfig struct {
	Ports         string
	Threads       int
	SocketTimeout time.Duration
	Timeout       time.Duration
}

func (config GogoConfig) validate() error {
	if err := validateGogoPorts(config.Ports); err != nil {
		return err
	}
	if config.Threads < 1 || config.Threads > 1000 {
		return errors.New("gogo threads must be between 1 and 1000")
	}
	if config.SocketTimeout < time.Second || config.SocketTimeout > 30*time.Second || config.SocketTimeout%time.Second != 0 {
		return errors.New("gogo socket timeout must be whole seconds between 1 and 30")
	}
	if config.Timeout < time.Minute || config.Timeout > 24*time.Hour || config.Timeout%time.Second != 0 {
		return errors.New("gogo timeout must be whole seconds between 60 and 86400")
	}
	return nil
}

func validateGogoPorts(value string) error {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 256 {
		return errors.New("gogo ports must be a bounded preset or numeric list")
	}
	for _, token := range strings.Split(value, ",") {
		switch token {
		case "top1", "top2", "top3", "common", "http", "in", "win", "db", "rce", "brute", "cloud", "info", "mail":
			continue
		}
		bounds := strings.Split(token, "-")
		if len(bounds) < 1 || len(bounds) > 2 {
			return fmt.Errorf("invalid gogo port token %q", token)
		}
		start, err := strconv.Atoi(bounds[0])
		if err != nil || start < 1 || start > 65535 {
			return fmt.Errorf("invalid gogo port token %q", token)
		}
		if len(bounds) == 2 {
			end, err := strconv.Atoi(bounds[1])
			if err != nil || end < start || end > 65535 || end-start > 1023 {
				return fmt.Errorf("invalid gogo port range %q", token)
			}
		}
	}
	return nil
}

type gogoPreparedTarget struct {
	args       []string
	hostsForIP func(net.IP) []string
}

type ipv4Lookup func(context.Context, string) ([]net.IP, error)

// RunGogo scans an IP/CIDR or resolved domain facts through official gogo's
// plain JSON Lines output. It deliberately omits exploit and active finger
// flags; the caller receives only validated HostPort and service candidates.
func RunGogo(ctx context.Context, binary string, target GogoTarget, subdomainsPath, workspace string, config GogoConfig, visit func(results.HostPort, *ServiceEndpoint) error) (ParseSummary, error) {
	if ctx == nil || binary == "" || workspace == "" || visit == nil {
		return ParseSummary{}, errors.New("gogo context, tool, workspace, and result visitor are required")
	}
	if err := config.validate(); err != nil {
		return ParseSummary{}, err
	}
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		return ParseSummary{}, errors.New("gogo workspace must be an existing directory")
	}
	workDir, err := os.MkdirTemp(workspace, "gogo-")
	if err != nil {
		return ParseSummary{}, fmt.Errorf("create gogo work directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	prepared, err := prepareGogoTarget(ctx, target, subdomainsPath, workDir, func(ctx context.Context, name string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip4", name)
	})
	if err != nil {
		return ParseSummary{}, err
	}
	if prepared == nil {
		return ParseSummary{}, nil
	}
	outputPath := filepath.Join(workDir, "results.json")
	args := append([]string{}, prepared.args...)
	args = append(args,
		"-p", config.Ports,
		"-t", strconv.Itoa(config.Threads),
		"-d", strconv.Itoa(int(config.SocketTimeout/time.Second)),
		"-D", strconv.Itoa(int(config.SocketTimeout/time.Second)),
		"-f", outputPath,
		"-O", "jl",
		"-C", "-q",
	)
	runCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, args...)
	command.Dir = workDir
	if err := command.Run(); err != nil {
		if runErr := runCtx.Err(); runErr != nil {
			return ParseSummary{}, fmt.Errorf("gogo execution: %w", runErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ParseSummary{}, fmt.Errorf("gogo exited with status %d", exitErr.ExitCode())
		}
		return ParseSummary{}, fmt.Errorf("start gogo: %w", err)
	}
	file, err := os.Open(outputPath)
	if err != nil {
		return ParseSummary{}, fmt.Errorf("open gogo result: %w", err)
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return ParseSummary{}, errors.New("gogo result must be a regular file")
	}
	return ParseGogoJSONLForHosts(ctx, file, prepared.hostsForIP, visit)
}

func prepareGogoTarget(ctx context.Context, target GogoTarget, subdomainsPath, workDir string, lookup ipv4Lookup) (*gogoPreparedTarget, error) {
	if lookup == nil {
		return nil, errors.New("gogo DNS resolver is required")
	}
	switch target.Type {
	case "ip":
		ip := net.ParseIP(target.Value)
		if ip == nil || ip.To4() == nil || ip.To4().String() != target.Value {
			return nil, errors.New("gogo Target IP must be canonical IPv4")
		}
		return &gogoPreparedTarget{args: []string{"-i", target.Value}, hostsForIP: func(candidate net.IP) []string {
			if candidate.Equal(ip) {
				return []string{target.Value}
			}
			return nil
		}}, nil
	case "cidr":
		_, network, err := net.ParseCIDR(target.Value)
		if err != nil || network.IP.To4() == nil {
			return nil, errors.New("gogo Target CIDR must be IPv4")
		}
		ones, bits := network.Mask.Size()
		if bits != 32 || uint64(1)<<uint(32-ones) > maximumGogoCandidates || network.String() != target.Value {
			return nil, errors.New("gogo Target CIDR is noncanonical or exceeds candidate limit")
		}
		return &gogoPreparedTarget{args: []string{"-i", target.Value}, hostsForIP: func(candidate net.IP) []string {
			if candidate.To4() != nil && network.Contains(candidate) {
				return []string{candidate.String()}
			}
			return nil
		}}, nil
	case "domain":
		return prepareGogoDomain(ctx, target.Value, subdomainsPath, workDir, lookup)
	default:
		return nil, fmt.Errorf("unsupported gogo Target type %q", target.Type)
	}
}

func prepareGogoDomain(ctx context.Context, domain, subdomainsPath, workDir string, lookup ipv4Lookup) (*gogoPreparedTarget, error) {
	canonical, valid := results.NormalizeSubdomainDNSName(domain)
	if !valid || canonical != domain {
		return nil, errors.New("gogo Target domain must be canonical")
	}
	if err := requireRegularFile(subdomainsPath); err != nil {
		return nil, fmt.Errorf("gogo subdomains input: %w", err)
	}
	file, err := os.Open(subdomainsPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	hosts := map[string]struct{}{domain: {}}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 4096)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := strings.TrimSpace(scanner.Text())
		if name == "" {
			continue
		}
		canonical, valid := results.NormalizeSubdomainDNSName(name)
		if !valid || canonical != name || (name != domain && !strings.HasSuffix(name, "."+domain)) {
			return nil, errors.New("gogo subdomain fact is outside Target scope")
		}
		hosts[name] = struct{}{}
		if len(hosts) > maximumGogoCandidates {
			return nil, errors.New("gogo domain host limit exceeded")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read gogo subdomains input: %w", err)
	}
	orderedHosts := make([]string, 0, len(hosts))
	for host := range hosts {
		orderedHosts = append(orderedHosts, host)
	}
	sort.Strings(orderedHosts)
	byIP := make(map[string][]string)
	for _, host := range orderedHosts {
		addresses, err := lookup(ctx, host)
		if err != nil {
			continue // One DNS failure does not invalidate other finalized names.
		}
		for _, address := range addresses {
			if ipv4 := address.To4(); ipv4 != nil {
				ip := ipv4.String()
				byIP[ip] = append(byIP[ip], host)
			}
		}
		if len(byIP) > maximumGogoCandidates {
			return nil, errors.New("gogo resolved IP limit exceeded")
		}
	}
	if len(byIP) == 0 {
		return nil, nil
	}
	orderedIPs := make([]string, 0, len(byIP))
	for ip := range byIP {
		orderedIPs = append(orderedIPs, ip)
	}
	sort.Strings(orderedIPs)
	inputPath := filepath.Join(workDir, "resolved-ips.txt")
	output, err := os.OpenFile(inputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create gogo resolved-IP input: %w", err)
	}
	for _, ip := range orderedIPs {
		if _, err := output.WriteString(ip + "\n"); err != nil {
			_ = output.Close()
			return nil, fmt.Errorf("write gogo resolved-IP input: %w", err)
		}
	}
	if err := output.Close(); err != nil {
		return nil, fmt.Errorf("close gogo resolved-IP input: %w", err)
	}
	return &gogoPreparedTarget{args: []string{"-l", inputPath}, hostsForIP: func(candidate net.IP) []string {
		return byIP[candidate.String()]
	}}, nil
}
