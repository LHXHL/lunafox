package chainreactor

import (
	"strings"
	"testing"
)

func TestZombieAuthResultPreservesWebOriginAndExcludesPassword(t *testing.T) {
	item, err := ZombieAuthResult(AuthFinding{Host: "auth.example.com", Port: 8443, Service: "https", Kind: "valid_credential", Account: "test"})
	if err != nil || item.URL != "https://auth.example.com:8443/" || item.Account != "test" || strings.Contains(strings.ToLower(item.URL), "password") {
		t.Fatalf("result = %+v, err = %v", item, err)
	}
	if _, err := ZombieAuthResult(AuthFinding{IP: "192.0.2.1", Port: 22, Service: "ssh", Kind: "valid_credential", Account: "test"}); err == nil {
		t.Fatal("non-Web zombie finding was accepted")
	}
}
