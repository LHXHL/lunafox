package zombieruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	enginecontract "github.com/yyhuni/lunafox/engines/zombie/contract"
)

// portServiceDefaults maps well-known ports to zombie plugin service names.
// Only services present here (and allowed by the services whitelist) become
// brute-force targets; unmapped ports are skipped and reported in progress.
var portServiceDefaults = map[int]string{
	21:     "ftp",
	22:     "ssh",
	110:    "pop3",
	139:    "smb",
	389:    "ldap",
	445:    "smb",
	873:    "rsync",
	1080:   "socks5",
	1433:   "mssql",
	1521:   "oracle",
	2181:   "zookeeper",
	3389:   "rdp",
	3306:   "mysql",
	5432:   "postgre",
	5672:   "mq",
	5900:   "vnc",
	6379:   "redis",
	11211:  "memcache",
	27017:  "mongo",
}

// hostPortFact mirrors one asset.host_port.v1 line of the mounted
// /run/lunafox/inputs/host-ports.jsonl fact file.
type hostPortFact struct {
	Host string `json:"host"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// zombieTargetPlan summarizes the derived per-service target file.
type zombieTargetPlan struct {
	Path          string
	Targets       uint64
	ServiceCounts map[string]uint64
	SkippedPorts  uint64
}

// prepareZombieTargets reads the HostPorts fact file, infers each port's
// service, filters by the configured whitelist, and writes one zombie URL
// target per line (`<service>://<ip>:<port>`, the form zombie's ParseUrl
// accepts). Unknown or non-whitelisted services are skipped.
func prepareZombieTargets(ctx context.Context, hostPortsPath, workDir string, allowedServices []string) (zombieTargetPlan, error) {
	plan := zombieTargetPlan{ServiceCounts: map[string]uint64{}}
	if ctx == nil {
		return plan, errors.New("target context is required")
	}
	if err := ctx.Err(); err != nil {
		return plan, err
	}
	if hostPortsPath == "" {
		return plan, errors.New("HostPorts facts path is required")
	}
	allowed := map[string]struct{}{}
	for _, service := range allowedServices {
		allowed[strings.ToLower(strings.TrimSpace(service))] = struct{}{}
	}
	if len(allowed) == 0 {
		return plan, errors.New("Zombie services whitelist must not be empty")
	}
	workspacePath, err := requireZombieWorkspace(workDir)
	if err != nil {
		return plan, err
	}
	source, err := os.Open(hostPortsPath)
	if err != nil {
		return plan, fmt.Errorf("open HostPorts facts: %w", err)
	}
	defer func() { _ = source.Close() }()

	plan.Path = filepath.Join(workspacePath, "zombie-targets.txt")
	file, err := os.OpenFile(plan.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return plan, fmt.Errorf("create zombie target file: %w", err)
	}
	cleanup := func(cause error) (zombieTargetPlan, error) {
		closeErr := file.Close()
		removeErr := os.Remove(plan.Path)
		if closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("close zombie target file: %w", closeErr))
		}
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			cause = errors.Join(cause, fmt.Errorf("remove incomplete zombie target file: %w", removeErr))
		}
		return zombieTargetPlan{ServiceCounts: map[string]uint64{}}, cause
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
		if line == "" {
			continue
		}
		var fact hostPortFact
		if err := json.Unmarshal([]byte(line), &fact); err != nil {
			return cleanup(fmt.Errorf("decode HostPorts fact: %w", err))
		}
		if fact.IP == "" {
			return cleanup(errors.New("HostPorts fact ip is required"))
		}
		if fact.Port < 1 || fact.Port > 65535 {
			return cleanup(fmt.Errorf("HostPorts fact port %d is out of range", fact.Port))
		}
		service, known := portServiceDefaults[fact.Port]
		if !known {
			plan.SkippedPorts++
			continue
		}
		if _, allowedService := allowed[service]; !allowedService {
			plan.SkippedPorts++
			continue
		}
		target := fmt.Sprintf("%s://%s:%d", service, fact.IP, fact.Port)
		if _, duplicate := seen[target]; duplicate {
			continue
		}
		seen[target] = struct{}{}
		if _, err := writer.WriteString(target + "\n"); err != nil {
			return cleanup(fmt.Errorf("write zombie target: %w", err))
		}
		plan.Targets++
		plan.ServiceCounts[service]++
	}
	if err := scanner.Err(); err != nil {
		return cleanup(fmt.Errorf("read HostPorts facts: %w", err))
	}
	if err := writer.Flush(); err != nil {
		return cleanup(fmt.Errorf("flush zombie targets: %w", err))
	}
	if err := file.Close(); err != nil {
		return zombieTargetPlan{ServiceCounts: map[string]uint64{}}, fmt.Errorf("close zombie target file: %w", err)
	}
	return plan, nil
}

// formatServiceCounts renders a stable "service=count" summary for progress.
func formatServiceCounts(counts map[string]uint64) string {
	services := make([]string, 0, len(counts))
	for service := range counts {
		services = append(services, service)
	}
	sort.Strings(services)
	parts := make([]string, 0, len(services))
	for _, service := range services {
		parts = append(parts, service+"="+strconv.FormatUint(counts[service], 10))
	}
	return strings.Join(parts, ",")
}

func requireZombieWorkspace(workspace string) (string, error) {
	if workspace == "" {
		return "", errors.New("Zombie workspace is required")
	}
	if workspace != strings.TrimSpace(workspace) {
		return "", errors.New("Zombie workspace must not have surrounding whitespace")
	}
	if !filepath.IsAbs(workspace) {
		return "", errors.New("Zombie workspace must be absolute")
	}
	if filepath.Clean(workspace) != workspace {
		return "", errors.New("Zombie workspace must be clean")
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return "", fmt.Errorf("inspect Zombie workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("Zombie workspace must be an existing directory")
	}
	return workspace, nil
}

// validateZombieConfig enforces the engine.json constraints before the tool runs.
func validateZombieConfig(config enginecontract.ZombieConfig) error {
	if !config.Enabled {
		return errors.New("Zombie section must be enabled")
	}
	if len(config.Services) == 0 {
		return errors.New("Zombie services whitelist must not be empty")
	}
	for _, service := range config.Services {
		if strings.TrimSpace(service) == "" {
			return errors.New("Zombie services whitelist must not contain blank entries")
		}
	}
	if config.Threads < 1 || config.Threads > 500 {
		return errors.New("Zombie threads must be between 1 and 500")
	}
	return nil
}
