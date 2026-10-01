package chainreactor

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunZombieOfficialBinaryLocalFixture(t *testing.T) {
	binary := os.Getenv("CHAINREACTOR_ZOMBIE_BINARY")
	if binary == "" {
		t.Skip("set CHAINREACTOR_ZOMBIE_BINARY to verify an official zombie release")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("test:testpass"))
		if request.Header.Get("Authorization") != want {
			writer.Header().Set("WWW-Authenticate", `Basic realm="local-test"`)
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte("local test success"))
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
	workspace := t.TempDir()
	users := filepath.Join(workspace, "users.txt")
	passwords := filepath.Join(workspace, "passwords.txt")
	if err := os.WriteFile(users, []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwords, []byte("testpass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var findings []AuthFinding
	summary, err := RunZombieLocalVerification(context.Background(), binary, []ServiceEndpoint{{IP: host, Port: port, Service: "http"}}, users, passwords, workspace, ZombieConfig{
		Timeout: time.Minute, RequestTimeout: 2 * time.Second, MaxAttempts: 1,
	}, func(item AuthFinding) error {
		findings = append(findings, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accepted != 1 || len(findings) != 1 || findings[0].Kind != "valid_credential" || findings[0].Account != "test" {
		t.Fatalf("unexpected zombie result: summary=%+v findings=%+v", summary, findings)
	}
	if strings.Contains(fmt.Sprint(findings), "testpass") {
		t.Fatal("password escaped the redacted finding")
	}
}

func TestRunZombieOfficialBinaryHostnameFixture(t *testing.T) {
	binary := os.Getenv("CHAINREACTOR_ZOMBIE_BINARY")
	if binary == "" {
		t.Skip("set CHAINREACTOR_ZOMBIE_BINARY to verify an official zombie release")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("test:testpass"))
		if !strings.HasPrefix(request.Host, "auth.localhost:") || request.Header.Get("Authorization") != want {
			writer.Header().Set("WWW-Authenticate", `Basic realm="local-test"`)
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	users := filepath.Join(workspace, "users.txt")
	passwords := filepath.Join(workspace, "passwords.txt")
	if err := os.WriteFile(users, []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwords, []byte("testpass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var findings []AuthFinding
	summary, err := RunZombieLocalVerification(context.Background(), binary, []ServiceEndpoint{{Host: "auth.localhost", Port: port, Service: "http"}}, users, passwords, workspace, ZombieConfig{
		Timeout: time.Minute, RequestTimeout: 2 * time.Second, MaxAttempts: 1,
	}, func(item AuthFinding) error {
		findings = append(findings, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accepted != 1 || len(findings) != 1 || findings[0].Host != "auth.localhost" || findings[0].IP != "" || strings.Contains(fmt.Sprint(findings), "testpass") {
		t.Fatalf("unexpected hostname zombie result: summary=%+v findings=%+v", summary, findings)
	}
}

func TestRunZombieRejectsAttemptBudgetBeforeProcess(t *testing.T) {
	workspace := t.TempDir()
	users := filepath.Join(workspace, "users.txt")
	passwords := filepath.Join(workspace, "passwords.txt")
	if err := os.WriteFile(users, []byte("a\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwords, []byte("x\ny\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := RunZombieLocalVerification(context.Background(), "/missing/zombie", []ServiceEndpoint{{IP: "127.0.0.1", Port: 22, Service: "ssh"}}, users, passwords, workspace, ZombieConfig{
		Timeout: time.Minute, RequestTimeout: time.Second, MaxAttempts: 3,
	}, func(AuthFinding) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "budget exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}
