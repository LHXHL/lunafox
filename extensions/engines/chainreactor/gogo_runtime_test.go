package chainreactor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yyhuni/lunafox/contracts/results"
)

func TestPrepareGogoDomainMapsSharedIPBackToEachInScopeHost(t *testing.T) {
	workspace := t.TempDir()
	input := filepath.Join(workspace, "subdomains.txt")
	if err := os.WriteFile(input, []byte("api.example.test\nwww.example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareGogoTarget(context.Background(), GogoTarget{Type: "domain", Value: "example.test"}, input, workspace,
		func(_ context.Context, name string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("192.0.2.10")}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if prepared == nil || len(prepared.args) != 2 || prepared.args[0] != "-l" {
		t.Fatalf("unexpected gogo input: %+v", prepared)
	}
	content, err := os.ReadFile(prepared.args[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "192.0.2.10\n" {
		t.Fatalf("unexpected resolved IP input: %q", content)
	}
	hosts := prepared.hostsForIP(net.ParseIP("192.0.2.10"))
	if strings.Join(hosts, ",") != "api.example.test,example.test,www.example.test" || len(prepared.hostsForIP(net.ParseIP("198.51.100.1"))) != 0 {
		t.Fatalf("unexpected host mapping: %v", hosts)
	}
}

func TestPrepareGogoDomainRejectsOutOfScopeFact(t *testing.T) {
	workspace := t.TempDir()
	input := filepath.Join(workspace, "subdomains.txt")
	if err := os.WriteFile(input, []byte("evil.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := prepareGogoTarget(context.Background(), GogoTarget{Type: "domain", Value: "example.test"}, input, workspace,
		func(context.Context, string) ([]net.IP, error) { t.Fatal("DNS should not run"); return nil, nil })
	if err == nil || !strings.Contains(err.Error(), "outside Target scope") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunGogoOfficialBinaryLocalFixture(t *testing.T) {
	binary := os.Getenv("CHAINREACTOR_GOGO_BINARY")
	if binary == "" {
		t.Skip("set CHAINREACTOR_GOGO_BINARY to verify an official gogo release")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte("gogo fixture\n"))
	}))
	defer server.Close()
	host, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	var got []results.HostPort
	summary, err := RunGogo(context.Background(), binary, GogoTarget{Type: "ip", Value: host}, "", t.TempDir(), GogoConfig{
		Ports: portText, Threads: 10, SocketTimeout: 2 * time.Second, Timeout: time.Minute,
	}, func(item results.HostPort, _ *ServiceEndpoint) error {
		got = append(got, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accepted == 0 || len(got) != 1 || got[0].IP != host || got[0].Port != port {
		t.Fatalf("unexpected official gogo result: summary=%+v items=%+v", summary, got)
	}
}

func TestGogoPortsRejectShellLikeValues(t *testing.T) {
	for _, value := range []string{"", "22;rm", "1-65535", "-e", "22,,80"} {
		if err := validateGogoPorts(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}
