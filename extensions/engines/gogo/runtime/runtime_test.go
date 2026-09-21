package gogoruntime

import (
	"encoding/json"
	"testing"

	enginecontract "github.com/yyhuni/lunafox/engines/gogo/contract"
)

func TestBuildGogoArgs(t *testing.T) {
	config := enginecontract.GogoConfig{
		Enabled:     true,
		PortTag:     "top2",
		Ports:       "",
		Concurrency: 600,
		Timeout:     3,
		Mod:         "default",
		Exploit:     true,
	}
	args, err := BuildGogoArgs(config, "/workspace/gogo-candidates.txt", "/workspace/gogo.jsonl")
	if err != nil {
		t.Fatalf("BuildGogoArgs: %v", err)
	}
	joined := joinArgs(args)
	for _, want := range []string{
		"--list /workspace/gogo-candidates.txt",
		"--port top2",
		"--thread 600",
		"--timeout 3",
		"--mod default",
		"--file /workspace/gogo.jsonl",
		"--file-output jl",
		"--compress",
		"--quiet",
		"--exploit",
	} {
		if !containsArg(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
}

func TestBuildGogoArgsExplicitPortsWin(t *testing.T) {
	config := enginecontract.GogoConfig{
		Enabled: true, PortTag: "top2", Ports: "80,8080-8090",
		Concurrency: 100, Timeout: 2, Mod: "ss",
	}
	args, err := BuildGogoArgs(config, "/c", "/o")
	if err != nil {
		t.Fatalf("BuildGogoArgs: %v", err)
	}
	if !containsArg(joinArgs(args), "--port 80,8080-8090") {
		t.Errorf("explicit ports must win: %s", joinArgs(args))
	}
}

func TestBuildGogoArgsRejectsBadMod(t *testing.T) {
	config := enginecontract.GogoConfig{Enabled: true, PortTag: "top1", Concurrency: 10, Timeout: 2, Mod: "bogus"}
	if _, err := BuildGogoArgs(config, "/c", "/o"); err == nil {
		t.Fatal("expected error for invalid mod")
	}
}

func TestParseGogoRecordHTTPFullMapping(t *testing.T) {
	line := `{"ip":"192.168.1.10","port":"8080","protocol":"http","status":"200","uri":"/index.html","host":"web.example.com","title":"Login","midware":"nginx","timing":45,"frameworks":{"thinkphp":{"name":"ThinkPHP"}},"vulns":{"thinkphp-rce":{"name":"ThinkPHP RCE","severity":4,"payload":{"path":"/index.php"}}}}`
	var record gogoRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	outcome, err := ParseGogoRecord(record)
	if err != nil {
		t.Fatalf("ParseGogoRecord: %v", err)
	}
	if outcome.HostPort == nil || outcome.HostPort.IP != "192.168.1.10" || outcome.HostPort.Port != 8080 || outcome.HostPort.Host != "web.example.com" {
		t.Errorf("hostPort = %+v", outcome.HostPort)
	}
	if outcome.Website == nil {
		t.Fatal("website is required for http protocol")
	}
	if outcome.Website.URL != "http://192.168.1.10:8080/index.html" {
		t.Errorf("website url = %q", outcome.Website.URL)
	}
	if outcome.Website.StatusCode == nil || *outcome.Website.StatusCode != 200 {
		t.Errorf("website status = %+v", outcome.Website.StatusCode)
	}
	if outcome.Website.Webserver != "nginx" || outcome.Website.Title != "Login" {
		t.Errorf("website = %+v", outcome.Website)
	}
	if outcome.Technology == nil || len(outcome.Technology.Tech) != 1 || outcome.Technology.Tech[0] != "ThinkPHP" {
		t.Errorf("technology = %+v", outcome.Technology)
	}
	if len(outcome.Vulnerabilities) != 1 {
		t.Fatalf("vulnerabilities = %d, want 1", len(outcome.Vulnerabilities))
	}
	vuln := outcome.Vulnerabilities[0]
	if vuln.Severity != "critical" || vuln.Source != "gogo" || vuln.VulnType != "ThinkPHP RCE" {
		t.Errorf("vulnerability = %+v", vuln)
	}
	if vuln.URL != "http://192.168.1.10:8080" {
		t.Errorf("vulnerability url = %q", vuln.URL)
	}
	if vuln.RawOutput == nil || vuln.RawOutput["protocol"] != "http" {
		t.Errorf("rawOutput = %+v", vuln.RawOutput)
	}
}

func TestParseGogoRecordNonHTTPFallbackURL(t *testing.T) {
	line := `{"ip":"10.0.0.5","port":"3306","protocol":"mysql","port_status":"open","vulns":{"mysql-weak":{"name":"weak password","severity":3}}}`
	var record gogoRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	outcome, err := ParseGogoRecord(record)
	if err != nil {
		t.Fatalf("ParseGogoRecord: %v", err)
	}
	if outcome.Website != nil {
		t.Error("mysql protocol must not produce a Website")
	}
	if len(outcome.Vulnerabilities) != 1 {
		t.Fatalf("vulnerabilities = %d", len(outcome.Vulnerabilities))
	}
	if outcome.Vulnerabilities[0].URL != "http://10.0.0.5:3306" {
		t.Errorf("fallback url = %q", outcome.Vulnerabilities[0].URL)
	}
	if outcome.Vulnerabilities[0].Severity != "high" {
		t.Errorf("severity = %q", outcome.Vulnerabilities[0].Severity)
	}
}

func TestParseGogoRecordRejectsBadPort(t *testing.T) {
	if _, err := ParseGogoRecord(gogoRecord{Ip: "1.2.3.4", Port: "http"}); err == nil {
		t.Fatal("expected error for non-numeric port")
	}
	if _, err := ParseGogoRecord(gogoRecord{Ip: "1.2.3.4", Port: "70000"}); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
	if _, err := ParseGogoRecord(gogoRecord{Port: "80"}); err == nil {
		t.Fatal("expected error for missing ip")
	}
}

func joinArgs(args []string) string {
	out := ""
	for i, arg := range args {
		if i > 0 {
			out += " "
		}
		out += arg
	}
	return out
}

func containsArg(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
