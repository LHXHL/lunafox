package chainreactor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/yyhuni/lunafox/contracts/results"
)

func TestParseGogoJSONLScopesAndKeepsConfirmedServices(t *testing.T) {
	input := strings.NewReader(`{"config":{"ip":"192.0.2.0/24"}}
{"ip":"192.0.2.10","port":"22","protocol":"tcp","frameworks":{"ssh":{"name":"ssh"}}}
{"ip":"192.0.2.10","port":"80","protocol":"http"}
{"ip":"198.51.100.3","port":"22","protocol":"ssh"}
{"ip":"192.0.2.10","port":"bad","protocol":"ssh"}
`)
	var ports []results.HostPort
	var services []ServiceEndpoint
	summary, err := ParseGogoJSONL(context.Background(), input, func(ip net.IP) bool {
		return ip.Equal(net.ParseIP("192.0.2.10"))
	}, func(port results.HostPort, service *ServiceEndpoint) error {
		ports = append(ports, port)
		if service != nil {
			services = append(services, *service)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary != (ParseSummary{Records: 5, Accepted: 2, Skipped: 3}) || len(ports) != 2 || len(services) != 1 || services[0].Service != "ssh" {
		t.Fatalf("unexpected gogo parse: summary=%+v ports=%+v services=%+v", summary, ports, services)
	}
}

func TestParseSprayJSONLMapsDirectoryWithoutEscapingScope(t *testing.T) {
	input := strings.NewReader(`{"url":"https://example.test/admin","status":403,"body_length":128,"spend":15}
{"url":"https://other.test/admin","status":200,"body_length":200,"spend":10}
{"url":"ftp://example.test/admin","status":200,"body_length":10,"spend":1}
{"url":"https://example.test/missing","status":200,"body_length":10}
`)
	var got []results.Directory
	summary, err := ParseSprayJSONL(context.Background(), input, func(u *url.URL) bool {
		return u.Hostname() == "example.test"
	}, func(item results.Directory) error {
		got = append(got, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accepted != 1 || summary.Skipped != 3 || len(got) != 1 || got[0].Duration != 15 || got[0].ContentLength != 128 {
		t.Fatalf("unexpected spray parse: summary=%+v items=%+v", summary, got)
	}
}

func TestZombieTargetLineContainsNoCredentials(t *testing.T) {
	line, err := ZombieTargetLine(ServiceEndpoint{IP: "192.0.2.10", Port: 22, Service: "ssh"})
	if err != nil || line != "ssh://192.0.2.10:22" {
		t.Fatalf("unexpected target: %q, %v", line, err)
	}
	if _, err := ZombieTargetLine(ServiceEndpoint{IP: "192.0.2.10", Port: 80, Service: "unknown"}); err == nil {
		t.Fatal("unconfirmed service should be rejected")
	}
	if line, err := ZombieTargetLine(ServiceEndpoint{IP: "2001:db8::10", Port: 5432, Service: "postgresql"}); err != nil || line != "postgresql://[2001:db8::10]:5432" {
		t.Fatalf("unexpected IPv6 target: %q, %v", line, err)
	}
	if line, err := ZombieTargetLine(ServiceEndpoint{Host: "auth.example.test", Port: 443, Service: "https"}); err != nil || line != "https://auth.example.test:443" {
		t.Fatalf("unexpected hostname target: %q, %v", line, err)
	}
	if _, err := ZombieTargetLine(ServiceEndpoint{Host: "auth.example.test", IP: "192.0.2.10", Port: 443, Service: "https"}); err == nil {
		t.Fatal("ambiguous hostname and IP target must be rejected")
	}
}

func TestGogoPostgreNameMapsToZombiePlugin(t *testing.T) {
	input := strings.NewReader(`{"ip":"192.0.2.10","port":5432,"protocol":"postgresql"}`)
	_, err := ParseGogoJSONL(context.Background(), input, func(net.IP) bool { return true }, func(_ results.HostPort, service *ServiceEndpoint) error {
		if service == nil || service.Service != "postgresql" {
			t.Fatalf("unexpected service: %+v", service)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseZombieJSONLNeverReturnsPassword(t *testing.T) {
	input := strings.NewReader(`{"ip":"192.0.2.10","port":"22","service":"ssh","username":"test","password":"secret","mod":0}
{"ip":"192.0.2.10","port":"22","service":"ssh","username":"","password":"","mod":1}
{"ip":"192.0.2.10","port":"22","service":"ssh","username":"test","password":"secret","mod":2}
{"ip":"198.51.100.1","port":"22","service":"ssh","username":"test","password":"secret","mod":0}
`)
	var findings []AuthFinding
	summary, err := ParseZombieJSONL(context.Background(), input, func(endpoint ServiceEndpoint) bool {
		return endpoint.IP == "192.0.2.10" && endpoint.Port == 22 && endpoint.Service == "ssh"
	}, func(item AuthFinding) error {
		findings = append(findings, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accepted != 2 || len(findings) != 2 || findings[0].Kind != "valid_credential" || findings[1].Kind != "anonymous_access" {
		t.Fatalf("unexpected zombie summary or findings: %+v %+v", summary, findings)
	}
	if strings.Contains(fmt.Sprintf("%+v", findings), "secret") {
		t.Fatal("plaintext password escaped the parser")
	}
}

func TestParseZombieJSONLMatchesExactHostnameTarget(t *testing.T) {
	input := strings.NewReader(`{"ip":"auth.example.test","port":"443","service":"https","username":"alice","password":"test-secret","mod":0}
{"ip":"other.example.test","port":"443","service":"https","username":"alice","password":"test-secret","mod":0}
`)
	var findings []AuthFinding
	summary, err := ParseZombieJSONL(context.Background(), input, func(endpoint ServiceEndpoint) bool {
		return endpoint == (ServiceEndpoint{Host: "auth.example.test", Port: 443, Service: "https"})
	}, func(item AuthFinding) error {
		findings = append(findings, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accepted != 1 || len(findings) != 1 || findings[0].Host != "auth.example.test" || findings[0].IP != "" || strings.Contains(fmt.Sprint(findings), "test-secret") {
		t.Fatalf("unexpected hostname finding: summary=%+v findings=%+v", summary, findings)
	}
}

func TestConflictingGogoFingerprintsDoNotFeedZombie(t *testing.T) {
	if got := confirmedService("tcp", map[string]json.RawMessage{"ssh": json.RawMessage(`{}`), "mysql": json.RawMessage(`{}`)}); got != "" {
		t.Fatalf("conflicting services must be rejected, got %q", got)
	}
}
