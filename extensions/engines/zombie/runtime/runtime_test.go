package zombieruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	enginecontract "github.com/yyhuni/lunafox/engines/zombie/contract"
)

func TestBuildZombieArgs(t *testing.T) {
	config := enginecontract.ZombieConfig{
		Enabled:  true,
		Services: []string{"ssh", "mysql"},
		Threads:  100,
	}
	args, err := BuildZombieArgs(config, "/workspace/zombie-targets.txt", "/workspace/zombie.jsonl")
	if err != nil {
		t.Fatalf("BuildZombieArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--IP /workspace/zombie-targets.txt",
		"--thread 100",
		"--file /workspace/zombie.jsonl",
		"--file-format json",
		"--quiet",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "--weakpass") {
		t.Errorf("weakpass must be off by default: %s", joined)
	}
}

func TestBuildZombieArgsWeakpassNeedsSeed(t *testing.T) {
	config := enginecontract.ZombieConfig{Enabled: true, Services: []string{"ssh"}, Threads: 10, Weakpass: true}
	args, err := BuildZombieArgs(config, "/t", "/o")
	if err != nil {
		t.Fatalf("BuildZombieArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--weakpass") || !strings.Contains(joined, "--pwd") {
		t.Errorf("weakpass requires a seed password: %s", joined)
	}
}

func TestBuildZombieArgsRejectsEmptyServices(t *testing.T) {
	config := enginecontract.ZombieConfig{Enabled: true, Services: nil, Threads: 10}
	if _, err := BuildZombieArgs(config, "/t", "/o"); err == nil {
		t.Fatal("expected error for empty services")
	}
}

func TestParseZombieRecord(t *testing.T) {
	record := zombieRecord{
		IP: "10.0.0.8", Port: "3306", Service: "mysql",
		Username: "root", Password: "root123", Scheme: "mysql", OK: true,
	}
	vulnerability, err := ParseZombieRecord(record)
	if err != nil {
		t.Fatalf("ParseZombieRecord: %v", err)
	}
	if vulnerability == nil {
		t.Fatal("vulnerability is required for ok record")
	}
	if vulnerability.URL != "http://10.0.0.8:3306" {
		t.Errorf("url = %q", vulnerability.URL)
	}
	if vulnerability.VulnType != "weak-credential:mysql" {
		t.Errorf("vulnType = %q", vulnerability.VulnType)
	}
	if vulnerability.Severity != "critical" || vulnerability.Source != "zombie" {
		t.Errorf("severity/source = %q/%q", vulnerability.Severity, vulnerability.Source)
	}
	raw := vulnerability.RawOutput
	if raw["username"] != "root" || raw["password"] != "root123" || raw["scheme"] != "mysql" {
		t.Errorf("rawOutput = %+v", raw)
	}
}

func TestParseZombieRecordSkipsFailed(t *testing.T) {
	vulnerability, err := ParseZombieRecord(zombieRecord{OK: false})
	if err != nil {
		t.Fatalf("ParseZombieRecord: %v", err)
	}
	if vulnerability != nil {
		t.Error("failed login must not produce a vulnerability")
	}
}

func TestParseZombieRecordRejectsMissingFields(t *testing.T) {
	if _, err := ParseZombieRecord(zombieRecord{OK: true, Port: "22", Service: "ssh", Username: "root"}); err == nil {
		t.Fatal("expected error for missing ip")
	}
	if _, err := ParseZombieRecord(zombieRecord{OK: true, IP: "1.2.3.4", Port: "22", Username: "root"}); err == nil {
		t.Fatal("expected error for missing service")
	}
	if _, err := ParseZombieRecord(zombieRecord{OK: true, IP: "1.2.3.4", Port: "22", Service: "ssh"}); err == nil {
		t.Fatal("expected error for missing username")
	}
}

func TestPrepareZombieTargetsFiltersAndMaps(t *testing.T) {
	dir := t.TempDir()
	factsPath := filepath.Join(dir, "host-ports.jsonl")
	facts := `{"host":"db.internal","ip":"10.0.0.8","port":3306}
{"host":"ssh.internal","ip":"10.0.0.9","port":22}
{"host":"web.internal","ip":"10.0.0.10","port":8080}
{"host":"redis.internal","ip":"10.0.0.11","port":6379}
`
	if err := os.WriteFile(factsPath, []byte(facts), 0o600); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	plan, err := prepareZombieTargets(context.Background(), factsPath, dir, []string{"ssh", "mysql"})
	if err != nil {
		t.Fatalf("prepareZombieTargets: %v", err)
	}
	if plan.Targets != 2 {
		t.Errorf("targets = %d, want 2 (mysql+ssh, redis filtered by whitelist, 8080 unmapped)", plan.Targets)
	}
	if plan.SkippedPorts != 2 {
		t.Errorf("skippedPorts = %d, want 2", plan.SkippedPorts)
	}
	content, err := os.ReadFile(plan.Path)
	if err != nil {
		t.Fatalf("read targets: %v", err)
	}
	got := string(content)
	for _, want := range []string{"mysql://10.0.0.8:3306\n", "ssh://10.0.0.9:22\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("targets missing %q: %q", want, got)
		}
	}
}

func TestPrepareZombieTargetsEmptyWhitelist(t *testing.T) {
	dir := t.TempDir()
	factsPath := filepath.Join(dir, "host-ports.jsonl")
	if err := os.WriteFile(factsPath, []byte(`{"host":"h","ip":"10.0.0.8","port":3306}`), 0o600); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	if _, err := prepareZombieTargets(context.Background(), factsPath, dir, nil); err == nil {
		t.Fatal("expected error for empty whitelist")
	}
}
