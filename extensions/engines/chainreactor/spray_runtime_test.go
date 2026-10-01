package chainreactor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yyhuni/lunafox/contracts/results"
)

func TestRunSprayConsumesCurrentWebsitesAndFiltersForeignOrigins(t *testing.T) {
	workspace := t.TempDir()
	websites := filepath.Join(workspace, "websites.txt")
	wordlist := filepath.Join(workspace, "words.txt")
	stub := filepath.Join(workspace, "spray-stub")
	for path, data := range map[string]string{
		websites: "https://example.test/\n",
		wordlist: "admin\n",
		stub: `#!/bin/sh
set -eu
test "$1" = '-l'
test "$3" = '-d'
test "$5" = '-m'
test "$6" = 'path'
test "$7" = '-O'
test "$8" = 'json'
test "$9" = '-f'
shift 9
output="$1"
cat > "$output" <<'JSON'
{"url":"https://example.test/","status":200,"body_length":100,"spend":1,"content_type":"html"}
{"url":"https://example.test/admin","status":403,"body_length":42,"spend":12}
{"url":"https://other.test/admin","status":200,"body_length":100,"spend":5}
JSON
`,
	} {
		mode := os.FileMode(0o600)
		if path == stub {
			mode = 0o700
		}
		if err := os.WriteFile(path, []byte(data), mode); err != nil {
			t.Fatal(err)
		}
	}
	var got []results.Directory
	summary, err := RunSpray(context.Background(), stub, websites, wordlist, workspace, SprayConfig{
		Pool: 1, ThreadsPerPool: 2, RatePerPool: 10, RequestTimeout: 5 * time.Second, Timeout: time.Minute,
	}, func(item results.Directory) error {
		got = append(got, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary != (ParseSummary{Records: 3, Accepted: 1, Skipped: 2}) || len(got) != 1 || got[0].URL != "https://example.test/admin" {
		t.Fatalf("summary=%+v results=%+v", summary, got)
	}
}

func TestRunSprayOfficialBinaryLocalFixture(t *testing.T) {
	binary := os.Getenv("CHAINREACTOR_SPRAY_BINARY")
	if binary == "" {
		t.Skip("set CHAINREACTOR_SPRAY_BINARY to verify an official spray release")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		if request.URL.Path == "/admin" || request.URL.Path == "/admin/" {
			_, _ = writer.Write([]byte("admin fixture\n"))
			return
		}
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte("missing\n"))
	}))
	defer server.Close()
	workspace := t.TempDir()
	websites := filepath.Join(workspace, "websites.txt")
	wordlist := filepath.Join(workspace, "words.txt")
	if err := os.WriteFile(websites, []byte(server.URL+"/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wordlist, []byte("admin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []results.Directory
	summary, err := RunSpray(context.Background(), binary, websites, wordlist, workspace, SprayConfig{
		Pool: 1, ThreadsPerPool: 2, RatePerPool: 10, RequestTimeout: 5 * time.Second, Timeout: time.Minute,
	}, func(item results.Directory) error {
		got = append(got, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accepted == 0 || len(got) == 0 {
		t.Fatalf("official spray produced no accepted directory: %+v", summary)
	}
	for _, item := range got {
		if !strings.HasPrefix(item.URL, server.URL+"/admin") || item.Status != 200 {
			t.Fatalf("unexpected result: %+v", item)
		}
	}
}

func TestRunSpraySkipsToolOnEmptyWebsiteInput(t *testing.T) {
	workspace := t.TempDir()
	websites := filepath.Join(workspace, "websites.txt")
	wordlist := filepath.Join(workspace, "words.txt")
	if err := os.WriteFile(websites, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wordlist, []byte("admin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := RunSpray(context.Background(), "/missing/spray", websites, wordlist, workspace, SprayConfig{
		Pool: 1, ThreadsPerPool: 1, RatePerPool: 10, RequestTimeout: time.Second, Timeout: time.Minute,
	}, func(results.Directory) error { t.Fatal("unexpected result"); return nil })
	if err != nil || summary != (ParseSummary{}) {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
}

func TestRunSprayRejectsUnsafeControls(t *testing.T) {
	_, err := RunSpray(context.Background(), "spray", "", "", t.TempDir(), SprayConfig{
		Pool: 20, ThreadsPerPool: 100, RatePerPool: 1000, RequestTimeout: time.Second, Timeout: time.Minute,
	}, func(results.Directory) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "limits exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}
