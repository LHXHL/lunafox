// Package chainreactor contains the bounded output adapters shared by the
// planned gogo, spray, and zombie Engine runtimes. It does not grant an Engine
// any result capability; the Server still validates every submitted result.
package chainreactor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/yyhuni/lunafox/contracts/results"
)

const maximumRecordBytes = 4 * 1024 * 1024

// ServiceEndpoint is an intermediate observation until the Server and Agent
// publish a typed ServiceEndpoints input contract.
type ServiceEndpoint struct {
	Host    string
	IP      string
	Port    int
	Service string
}

// AuthFinding is deliberately credential-free. Zombie's JSON output includes
// plaintext passwords; those bytes never enter this value or a result port.
type AuthFinding struct {
	Host    string
	IP      string
	Port    int
	Service string
	Kind    string
	Account string
}

type ParseSummary struct {
	Records  int
	Accepted int
	Skipped  int
}

// ParseGogoJSONL reads gogo's -O jl output. Only in-scope IPs can reach the
// caller, and a protocol becomes a service candidate only when it is explicit.
func ParseGogoJSONL(ctx context.Context, input io.Reader, inScope func(net.IP) bool, visit func(results.HostPort, *ServiceEndpoint) error) (ParseSummary, error) {
	if inScope == nil || visit == nil {
		return ParseSummary{}, errors.New("scope predicate and result visitor are required")
	}
	return ParseGogoJSONLForHosts(ctx, input, func(ip net.IP) []string {
		if !inScope(ip) {
			return nil
		}
		return []string{ip.String()}
	}, visit)
}

// ParseGogoJSONLForHosts projects each scanned IP back to the canonical host
// names selected before running gogo. Domain targets require this association
// because gogo's output contains the resolved IP, while LunaFox scopes a
// HostPort under the original DNS name.
func ParseGogoJSONLForHosts(ctx context.Context, input io.Reader, hostsForIP func(net.IP) []string, visit func(results.HostPort, *ServiceEndpoint) error) (ParseSummary, error) {
	if hostsForIP == nil || visit == nil {
		return ParseSummary{}, errors.New("host mapping and result visitor are required")
	}
	return parseLines(ctx, input, func(line []byte) (bool, error) {
		var record struct {
			IP         string                     `json:"ip"`
			Port       json.RawMessage            `json:"port"`
			Protocol   string                     `json:"protocol"`
			Frameworks map[string]json.RawMessage `json:"frameworks"`
		}
		if json.Unmarshal(line, &record) != nil {
			return false, nil
		}
		ip := net.ParseIP(record.IP)
		port, ok := parsePort(record.Port)
		if ip == nil || !ok {
			return false, nil
		}
		hosts := hostsForIP(ip)
		if len(hosts) == 0 {
			return false, nil
		}
		var service *ServiceEndpoint
		if name := confirmedService(record.Protocol, record.Frameworks); name != "" {
			service = &ServiceEndpoint{IP: ip.String(), Port: port, Service: name}
		}
		accepted := false
		for _, host := range hosts {
			item := results.HostPort{Host: host, IP: ip.String(), Port: port}
			if _, err := results.EncodeHostPort(item); err != nil {
				continue
			}
			if err := visit(item, service); err != nil {
				return accepted, err
			}
			accepted = true
		}
		return accepted, nil
	})
}

// ParseSprayJSONL reads spray's -O json file, which contains one observation
// per line. The tool has already filtered invalid fuzz matches in that file.
func ParseSprayJSONL(ctx context.Context, input io.Reader, inScope func(*url.URL) bool, visit func(results.Directory) error) (ParseSummary, error) {
	if inScope == nil || visit == nil {
		return ParseSummary{}, errors.New("scope predicate and result visitor are required")
	}
	return parseLines(ctx, input, func(line []byte) (bool, error) {
		var record struct {
			URL         *string `json:"url"`
			Status      *int    `json:"status"`
			BodyLength  *int64  `json:"body_length"`
			Spend       *int64  `json:"spend"`
			ContentType string  `json:"content_type"`
			Valid       *bool   `json:"valid"`
			Error       string  `json:"error"`
		}
		if json.Unmarshal(line, &record) != nil || record.URL == nil || record.Status == nil || record.BodyLength == nil || record.Spend == nil || (record.Valid != nil && !*record.Valid) || record.Error != "" {
			return false, nil
		}
		parsed, err := url.Parse(*record.URL)
		if err != nil || !inScope(parsed) {
			return false, nil
		}
		item := results.Directory{URL: *record.URL, Status: *record.Status, ContentLength: *record.BodyLength, ContentType: record.ContentType, Duration: *record.Spend}
		if _, err := results.EncodeDirectory(item); err != nil {
			return false, nil
		}
		return true, visit(item)
	})
}

// ZombieTargetLine emits a credential-free target for zombie -I. It rejects
// guessed or unsupported service names and never embeds an account or secret.
func ZombieTargetLine(endpoint ServiceEndpoint) (string, error) {
	if endpoint.Port < 1 || endpoint.Port > 65535 || (knownService(endpoint.Service) != endpoint.Service && endpoint.Service != "http" && endpoint.Service != "https") {
		return "", errors.New("invalid zombie service endpoint")
	}
	address := endpoint.IP
	if endpoint.Host != "" {
		canonical, valid := results.NormalizeSubdomainDNSName(endpoint.Host)
		if endpoint.IP != "" || !valid || canonical != endpoint.Host {
			return "", errors.New("invalid zombie service endpoint host")
		}
		address = endpoint.Host
	} else {
		ip := net.ParseIP(endpoint.IP)
		if ip == nil || ip.String() != endpoint.IP {
			return "", errors.New("invalid zombie service endpoint IP")
		}
	}
	return endpoint.Service + "://" + net.JoinHostPort(address, strconv.Itoa(endpoint.Port)), nil
}

// ParseZombieJSONL accepts successful brute or unauthenticated observations
// from zombie -O json. Mode 2 is an uncertain honeypot/check signal and is
// intentionally omitted. Passwords and auxiliary output never reach visit.
func ParseZombieJSONL(ctx context.Context, input io.Reader, inScope func(ServiceEndpoint) bool, visit func(AuthFinding) error) (ParseSummary, error) {
	if inScope == nil || visit == nil {
		return ParseSummary{}, errors.New("scope predicate and finding visitor are required")
	}
	return parseLines(ctx, input, func(line []byte) (bool, error) {
		var record struct {
			IP       string          `json:"ip"`
			Port     json.RawMessage `json:"port"`
			Service  string          `json:"service"`
			Username string          `json:"username"`
			Password json.RawMessage `json:"password"`
			Mod      *int            `json:"mod"`
		}
		if json.Unmarshal(line, &record) != nil || record.Mod == nil {
			return false, nil
		}
		port, validPort := parsePort(record.Port)
		if !validPort || (knownService(record.Service) != record.Service && record.Service != "http" && record.Service != "https") {
			return false, nil
		}
		endpoint := ServiceEndpoint{Port: port, Service: record.Service}
		if ip := net.ParseIP(record.IP); ip != nil {
			endpoint.IP = ip.String()
		} else if canonical, valid := results.NormalizeSubdomainDNSName(record.IP); valid && canonical == record.IP {
			endpoint.Host = record.IP
		} else {
			return false, nil
		}
		if !inScope(endpoint) {
			return false, nil
		}
		finding := AuthFinding{Host: endpoint.Host, IP: endpoint.IP, Port: endpoint.Port, Service: endpoint.Service, Account: record.Username}
		switch *record.Mod {
		case 0: // parsers.ZombieModBrute
			if len(record.Password) == 0 {
				return false, nil
			}
			var discarded string
			if json.Unmarshal(record.Password, &discarded) != nil {
				return false, nil
			}
			finding.Kind = "valid_credential"
		case 1: // parsers.ZombieModUnauth
			finding.Kind = "anonymous_access"
			finding.Account = ""
		default:
			return false, nil
		}
		return true, visit(finding)
	})
}

func knownService(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "postgres", "postgresql", "postgre":
		return "postgresql"
	case "mongodb", "mongo":
		return "mongo"
	case "mariadb":
		return "mysql"
	case "sqlserver":
		return "mssql"
	case "ssh", "mysql", "mssql", "redis", "ftp", "smb":
		return strings.ToLower(strings.TrimSpace(protocol))
	default:
		return ""
	}
}

func confirmedService(protocol string, frameworks map[string]json.RawMessage) string {
	service := knownService(protocol)
	for name := range frameworks {
		candidate := knownService(name)
		if candidate == "" {
			continue
		}
		if service != "" && service != candidate {
			return ""
		}
		service = candidate
	}
	return service
}

func parsePort(raw json.RawMessage) (int, bool) {
	var number int
	if json.Unmarshal(raw, &number) == nil {
		return number, number >= 1 && number <= 65535
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	number, err := strconv.Atoi(text)
	return number, err == nil && number >= 1 && number <= 65535
}

func parseLines(ctx context.Context, input io.Reader, visit func([]byte) (bool, error)) (ParseSummary, error) {
	if ctx == nil || input == nil {
		return ParseSummary{}, errors.New("context and input are required")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maximumRecordBytes)
	var summary ParseSummary
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		summary.Records++
		accepted, err := visit(line)
		if err != nil {
			return summary, err
		}
		if accepted {
			summary.Accepted++
		} else {
			summary.Skipped++
		}
	}
	if err := scanner.Err(); err != nil {
		return summary, fmt.Errorf("read ChainReactor output (record limit %d bytes): %w", maximumRecordBytes, err)
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	return summary, nil
}
