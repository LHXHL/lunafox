package chainreactor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestZombieWebTargetsPreservesHostnameAndDeduplicatesOrigins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "website-urls.txt")
	if err := os.WriteFile(path, []byte("https://auth.example.test/\nhttps://auth.example.test/\nhttp://127.0.0.1:8080/\nhttps://auth.example.test/admin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ZombieWebTargets(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	want := []ServiceEndpoint{
		{IP: "127.0.0.1", Port: 8080, Service: "http"},
		{Host: "auth.example.test", Port: 443, Service: "https"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected zombie website targets: got=%+v want=%+v", got, want)
	}
}

func TestZombieWebTargetsRejectsInvalidInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "website-urls.txt")
	if err := os.WriteFile(path, []byte("https://valid.example.test/\nhttps://other.example.test:70000/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ZombieWebTargets(context.Background(), path); err == nil {
		t.Fatal("invalid URL input should fail")
	}
}
