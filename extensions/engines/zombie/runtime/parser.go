package zombieruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	enginecontract "github.com/yyhuni/lunafox/engines/zombie/contract"
)

// zombieRecord mirrors the zombie v1.3 `-O json` file-output schema (one
// flattened Result/ZombieResult JSON object per line, written only for
// successful logins by core/runner.go OutputHandler).
type zombieRecord struct {
	IP       string `json:"ip"`
	Port     string `json:"port"`
	Service  string `json:"service"`
	Username string `json:"username"`
	Password string `json:"password"`
	Scheme   string `json:"scheme"`
	OK       bool   `json:"ok"`
	ErrString string `json:"error"`
}

// ParseZombieRecord converts one decoded zombie record into a canonical
// Vulnerability observation. A successful login is reported as a critical
// weak-credential finding; the canonical URL schema only accepts http(s), so
// the real service scheme travels in vulnType and rawOutput.
func ParseZombieRecord(record zombieRecord) (*enginecontract.Vulnerability, error) {
	if !record.OK {
		return nil, nil
	}
	if record.IP == "" {
		return nil, errors.New("zombie record ip is required")
	}
	port := strings.TrimSpace(record.Port)
	if port == "" {
		return nil, errors.New("zombie record port is required")
	}
	service := strings.ToLower(strings.TrimSpace(record.Service))
	if service == "" {
		return nil, errors.New("zombie record service is required")
	}
	if record.Username == "" {
		return nil, errors.New("zombie record username is required")
	}
	scheme := record.Scheme
	if scheme == "" {
		scheme = service
	}
	vulnerabilityType := "weak-credential:" + service
	return &enginecontract.Vulnerability{
		URL:      fmt.Sprintf("http://%s:%s", record.IP, port),
		VulnType: vulnerabilityType,
		Severity: "critical",
		Source:   "zombie",
		Description: fmt.Sprintf(
			"valid %s credential on %s:%s (username %q)",
			service, record.IP, port, record.Username,
		),
		RawOutput: map[string]any{
			"ip":       record.IP,
			"port":     port,
			"service":  service,
			"scheme":   scheme,
			"username": record.Username,
			"password": record.Password,
		},
	}, nil
}

// zombieParseSummary counts the consumed raw records for progress reporting.
type zombieParseSummary struct {
	Records        uint64
	Vulnerabilities uint64
}

// parseZombieOutput streams the zombie JSONL artifact, converts successful
// logins, and submits typed Vulnerability results with exact-key dedup.
func parseZombieOutput(ctx context.Context, artifact io.Reader, results enginecontract.Results) (zombieParseSummary, error) {
	summary := zombieParseSummary{}
	if results.Vulnerabilities == nil {
		return summary, errors.New("typed Vulnerability result port is required")
	}
	vulnerabilityCh := make(chan enginecontract.Vulnerability, 64)
	vulnerabilityDone := make(chan error, 1)
	go func() {
		vulnerabilityDone <- results.Vulnerabilities.Submit(ctx, vulnerabilityCh)
	}()

	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(artifact)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			close(vulnerabilityCh)
			return summary, err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record zombieRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			close(vulnerabilityCh)
			return summary, fmt.Errorf("decode zombie record %d: %w", summary.Records+1, err)
		}
		summary.Records++
		vulnerability, err := ParseZombieRecord(record)
		if err != nil {
			close(vulnerabilityCh)
			return summary, fmt.Errorf("convert zombie record %d: %w", summary.Records, err)
		}
		if vulnerability == nil {
			continue
		}
		key := vulnerability.URL + "|" + vulnerability.VulnType + "|" + record.Username
		if _, duplicate := seen[key]; !duplicate {
			seen[key] = struct{}{}
			select {
			case vulnerabilityCh <- *vulnerability:
				summary.Vulnerabilities++
			case <-ctx.Done():
				close(vulnerabilityCh)
				return summary, ctx.Err()
			}
		}
	}
	if err := scanner.Err(); err != nil {
		close(vulnerabilityCh)
		return summary, fmt.Errorf("read zombie artifact: %w", err)
	}
	close(vulnerabilityCh)
	if err := <-vulnerabilityDone; err != nil {
		return summary, fmt.Errorf("submit Vulnerability results: %w", err)
	}
	return summary, nil
}
