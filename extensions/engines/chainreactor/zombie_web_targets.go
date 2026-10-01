package chainreactor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/yyhuni/lunafox/contracts/results"
)

// ZombieWebTargets consumes the existing WebsiteURLs input role. A website
// origin preserves the hostname needed for Host/SNI-aware HTTP authentication;
// HostPorts alone cannot provide that binding for a domain Target.
func ZombieWebTargets(ctx context.Context, websiteURLsPath string) ([]ServiceEndpoint, error) {
	if ctx == nil {
		return nil, errors.New("zombie website target context is required")
	}
	if err := requireRegularFile(websiteURLsPath); err != nil {
		return nil, fmt.Errorf("zombie website URLs input: %w", err)
	}
	file, err := os.Open(websiteURLsPath)
	if err != nil {
		return nil, fmt.Errorf("open zombie website URLs input: %w", err)
	}
	defer file.Close()

	seen := make(map[ServiceEndpoint]struct{})
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 4096)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value := strings.TrimSpace(scanner.Text())
		if value == "" {
			continue
		}
		if _, err := results.ValidateObservedAssetURL(value); err != nil {
			return nil, fmt.Errorf("invalid zombie website URL input: %w", err)
		}
		parsed, err := url.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("parse zombie website URL input: %w", err)
		}
		if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			continue // A path-specific website is not equivalent to origin authentication.
		}
		port := 80
		if parsed.Scheme == "https" {
			port = 443
		}
		if parsed.Port() != "" {
			port, err = strconv.Atoi(parsed.Port())
			if err != nil || port < 1 || port > 65535 {
				return nil, errors.New("zombie website URL port is invalid")
			}
		}
		endpoint := ServiceEndpoint{Port: port, Service: parsed.Scheme}
		host := parsed.Hostname()
		if ip := net.ParseIP(host); ip != nil {
			endpoint.IP = ip.String()
		} else if canonical, valid := results.NormalizeSubdomainDNSName(host); valid && canonical == host {
			endpoint.Host = host
		} else {
			return nil, errors.New("zombie website hostname is invalid")
		}
		if _, err := ZombieTargetLine(endpoint); err != nil {
			return nil, err
		}
		seen[endpoint] = struct{}{}
		if len(seen) > 1000 {
			return nil, errors.New("zombie website target limit exceeded")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read zombie website URLs input: %w", err)
	}
	endpoints := make([]ServiceEndpoint, 0, len(seen))
	for endpoint := range seen {
		endpoints = append(endpoints, endpoint)
	}
	sort.Slice(endpoints, func(left, right int) bool {
		first, _ := ZombieTargetLine(endpoints[left])
		second, _ := ZombieTargetLine(endpoints[right])
		return first < second
	})
	return endpoints, nil
}
