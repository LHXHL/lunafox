package gogoruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	enginecontract "github.com/yyhuni/lunafox/engines/gogo/contract"
)

// gogoRecord mirrors the gogo v2.15 `-O jl -C` file-output schema (one
// parsers.GOGOResult JSON object per line, written by core/output.go).
type gogoRecord struct {
	Ip         string `json:"ip"`
	Port       string `json:"port"`
	Protocol   string `json:"protocol"`
	Status     string `json:"status"`
	Uri        string `json:"uri"`
	Host       string `json:"host"`
	Title      string `json:"title"`
	Midware    string `json:"midware"`
	Timing     int64  `json:"timing"`
	Frameworks map[string]struct {
		Name string `json:"name"`
	} `json:"frameworks"`
	Vulns map[string]struct {
		Name     string         `json:"name"`
		Severity int            `json:"severity"`
		Payload  map[string]any `json:"payload"`
		Detail   map[string][]string `json:"detail"`
	} `json:"vulns"`
}

// gogoSeverityLevels mirrors chainreactors/utils/parsers.Severity* constants.
var gogoSeverityLevels = map[int]string{
	1: "info",
	2: "medium",
	3: "high",
	4: "critical",
	5: "unknown",
}

// gogoParseOutcome carries the canonical items derived from one gogo record.
type gogoParseOutcome struct {
	HostPort     *enginecontract.HostPort
	Website      *enginecontract.Website
	Technology   *enginecontract.WebsiteTechnology
	Vulnerabilities []enginecontract.Vulnerability
}

// ParseGogoRecord converts one decoded gogo record into canonical typed
// results. Every open line yields a HostPort; http(s) protocols additionally
// yield a Website; frameworks yield a WebsiteTechnology; each vuln entry
// yields a Vulnerability. Non-http protocols fall back to an http:// URL
// because the canonical vulnerability schema only accepts http(s) URLs; the
// real protocol stays in rawOutput.
func ParseGogoRecord(record gogoRecord) (gogoParseOutcome, error) {
	outcome := gogoParseOutcome{}
	if record.Ip == "" {
		return outcome, errors.New("gogo record ip is required")
	}
	port, err := strconv.Atoi(strings.TrimSpace(record.Port))
	if err != nil {
		return outcome, fmt.Errorf("gogo record port %q is not numeric", record.Port)
	}
	if port < 1 || port > 65535 {
		return outcome, fmt.Errorf("gogo record port %d is out of range", port)
	}
	host := record.Host
	if host == "" {
		host = record.Ip
	}
	outcome.HostPort = &enginecontract.HostPort{Host: host, IP: record.Ip, Port: port}

	protocol := strings.ToLower(strings.TrimSpace(record.Protocol))
	isHTTP := protocol == "http" || protocol == "https"
	baseURL := fmt.Sprintf("http://%s:%d", record.Ip, port)
	if isHTTP {
		baseURL = fmt.Sprintf("%s://%s:%d", protocol, record.Ip, port)
	}
	uri := record.Uri
	if uri == "" {
		uri = "/"
	}

	if isHTTP {
		website := &enginecontract.Website{
			URL:    baseURL + uri,
			Host:   host,
			Title:  record.Title,
			Webserver: record.Midware,
		}
		if statusCode, convErr := strconv.Atoi(strings.TrimSpace(record.Status)); convErr == nil && statusCode >= 100 && statusCode <= 599 {
			website.StatusCode = &statusCode
		}
		outcome.Website = website
	}

	if len(record.Frameworks) > 0 {
		names := make([]string, 0, len(record.Frameworks))
		for key, framework := range record.Frameworks {
			name := framework.Name
			if name == "" {
				name = key
			}
			if name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			technologyURL := baseURL + uri
			if !isHTTP {
				technologyURL = baseURL
			}
			outcome.Technology = &enginecontract.WebsiteTechnology{URL: technologyURL, Tech: names}
		}
	}

	for key, vuln := range record.Vulns {
		name := vuln.Name
		if name == "" {
			name = key
		}
		if name == "" {
			continue
		}
		severity, ok := gogoSeverityLevels[vuln.Severity]
		if !ok {
			severity = "unknown"
		}
		raw := map[string]any{
			"ip":       record.Ip,
			"port":     port,
			"protocol": protocol,
			"source":   "gogo",
		}
		if len(vuln.Payload) > 0 {
			raw["payload"] = vuln.Payload
		}
		if len(vuln.Detail) > 0 {
			raw["detail"] = vuln.Detail
		}
		outcome.Vulnerabilities = append(outcome.Vulnerabilities, enginecontract.Vulnerability{
			URL:         baseURL,
			VulnType:    name,
			Severity:    severity,
			Source:      "gogo",
			Description: fmt.Sprintf("gogo neutron POC %s hit on %s:%d (%s)", name, record.Ip, port, protocol),
			RawOutput:   raw,
		})
	}
	return outcome, nil
}

// gogoParseSummary counts the submitted items per port for progress reporting.
type gogoParseSummary struct {
	Records         uint64
	HostPorts       uint64
	Websites        uint64
	Technologies    uint64
	Vulnerabilities uint64
}

// parseGogoOutput streams the gogo JSONL artifact, converts records, and
// submits typed results through the canonical ports with exact-key dedup.
func parseGogoOutput(ctx context.Context, artifact io.Reader, results enginecontract.Results) (gogoParseSummary, error) {
	summary := gogoParseSummary{}
	if results.HostPorts == nil {
		return summary, errors.New("typed HostPort result port is required")
	}
	hostPortCh := make(chan enginecontract.HostPort, 64)
	hostPortDone := make(chan error, 1)
	go func() { hostPortDone <- results.HostPorts.Submit(ctx, hostPortCh) }()
	websiteCh := make(chan enginecontract.Website, 64)
	websiteDone := make(chan error, 1)
	if results.Websites != nil {
		go func() { websiteDone <- results.Websites.Submit(ctx, websiteCh) }()
	} else {
		close(websiteCh)
		websiteDone <- nil
	}
	technologyCh := make(chan enginecontract.WebsiteTechnology, 64)
	technologyDone := make(chan error, 1)
	if results.WebsiteTechnologies != nil {
		go func() { technologyDone <- results.WebsiteTechnologies.Submit(ctx, technologyCh) }()
	} else {
		close(technologyCh)
		technologyDone <- nil
	}
	vulnerabilityCh := make(chan enginecontract.Vulnerability, 64)
	vulnerabilityDone := make(chan error, 1)
	if results.Vulnerabilities != nil {
		go func() { vulnerabilityDone <- results.Vulnerabilities.Submit(ctx, vulnerabilityCh) }()
	} else {
		close(vulnerabilityCh)
		vulnerabilityDone <- nil
	}

	abort := func(summary gogoParseSummary, cause error) (gogoParseSummary, error) {
		close(hostPortCh)
		close(websiteCh)
		close(technologyCh)
		close(vulnerabilityCh)
		return summary, cause
	}

	seenHostPort := make(map[string]struct{})
	seenWebsite := make(map[string]struct{})
	seenTechnology := make(map[string]struct{})
	seenVulnerability := make(map[string]struct{})
	scanner := bufio.NewScanner(artifact)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return abort(summary, err)
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record gogoRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return abort(summary, fmt.Errorf("decode gogo record %d: %w", summary.Records+1, err))
		}
		// The jl file opens (and may close) with a parameter-echo line that
		// carries no port; only port-bearing lines are scan results.
		if strings.TrimSpace(record.Port) == "" {
			continue
		}
		summary.Records++
		outcome, err := ParseGogoRecord(record)
		if err != nil {
			return abort(summary, fmt.Errorf("convert gogo record %d: %w", summary.Records, err))
		}
		if outcome.HostPort != nil {
			key := fmt.Sprintf("%s|%s|%d", outcome.HostPort.Host, outcome.HostPort.IP, outcome.HostPort.Port)
			if _, duplicate := seenHostPort[key]; !duplicate {
				seenHostPort[key] = struct{}{}
				select {
				case hostPortCh <- *outcome.HostPort:
					summary.HostPorts++
				case <-ctx.Done():
					return abort(summary, ctx.Err())
				}
			}
		}
		if outcome.Website != nil {
			if _, duplicate := seenWebsite[outcome.Website.URL]; !duplicate {
				seenWebsite[outcome.Website.URL] = struct{}{}
				select {
				case websiteCh <- *outcome.Website:
					summary.Websites++
				case <-ctx.Done():
					return abort(summary, ctx.Err())
				}
			}
		}
		if outcome.Technology != nil {
			if _, duplicate := seenTechnology[outcome.Technology.URL]; !duplicate {
				seenTechnology[outcome.Technology.URL] = struct{}{}
				select {
				case technologyCh <- *outcome.Technology:
					summary.Technologies++
				case <-ctx.Done():
					return abort(summary, ctx.Err())
				}
			}
		}
		for _, vulnerability := range outcome.Vulnerabilities {
			key := vulnerability.URL + "|" + vulnerability.VulnType
			if _, duplicate := seenVulnerability[key]; !duplicate {
				seenVulnerability[key] = struct{}{}
				select {
				case vulnerabilityCh <- vulnerability:
					summary.Vulnerabilities++
				case <-ctx.Done():
					return abort(summary, ctx.Err())
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return abort(summary, fmt.Errorf("read gogo artifact: %w", err))
	}
	close(hostPortCh)
	close(websiteCh)
	close(technologyCh)
	close(vulnerabilityCh)
	if err := <-hostPortDone; err != nil {
		return summary, fmt.Errorf("submit HostPort results: %w", err)
	}
	if err := <-websiteDone; err != nil {
		return summary, fmt.Errorf("submit Website results: %w", err)
	}
	if err := <-technologyDone; err != nil {
		return summary, fmt.Errorf("submit WebsiteTechnology results: %w", err)
	}
	if err := <-vulnerabilityDone; err != nil {
		return summary, fmt.Errorf("submit Vulnerability results: %w", err)
	}
	return summary, nil
}
