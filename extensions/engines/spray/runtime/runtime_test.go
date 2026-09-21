package sprayruntime

import (
	"testing"

	enginecontract "github.com/yyhuni/lunafox/engines/spray/contract"
)

func TestBuildSprayArgs(t *testing.T) {
	config := enginecontract.SprayConfig{
		Enabled:        true,
		Wordlist:       "/run/lunafox/resources/config/spray/wordlist/dir_default.txt",
		Pool:           5,
		Threads:        20,
		RequestTimeout: 5,
		Mod:            "path",
	}
	args, err := BuildSprayArgs(config, "/workspace/spray-candidates.txt", "/workspace/spray.jsonl")
	if err != nil {
		t.Fatalf("BuildSprayArgs: %v", err)
	}
	joined := argsString(args)
	for _, want := range []string{
		"--list /workspace/spray-candidates.txt",
		"--dict /run/lunafox/resources/config/spray/wordlist/dir_default.txt",
		"--pool 5",
		"--thread 20",
		"--timeout 5",
		"--mod path",
		"--file /workspace/spray.jsonl",
		"--file-output json",
		"--quiet",
	} {
		if !contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
}

func TestBuildSprayArgsRejectsDisabled(t *testing.T) {
	if _, err := BuildSprayArgs(enginecontract.SprayConfig{Enabled: false}, "/c", "/o"); err == nil {
		t.Fatal("expected error for disabled section")
	}
}

func TestParseSprayRecord(t *testing.T) {
	record := sprayRecord{
		IsValid:     true,
		UrlString:   "https://example.com/admin/",
		Host:        "example.com",
		BodyLength:  512,
		Status:      200,
		Spended:     42,
		ContentType: "text/html",
		Title:       "Admin",
		Frameworks: map[string]struct {
			Name string `json:"name"`
		}{
			"nginx": {Name: "nginx"},
			"thinkphp": {
				Name: "",
			},
		},
	}
	directories, technologies, err := ParseSprayRecord(record)
	if err != nil {
		t.Fatalf("ParseSprayRecord: %v", err)
	}
	if len(directories) != 1 {
		t.Fatalf("directories = %d, want 1", len(directories))
	}
	got := directories[0]
	if got.URL != record.UrlString || got.Status != 200 || got.ContentLength != 512 || got.ContentType != "text/html" || got.Duration != 42 {
		t.Errorf("directory = %+v", got)
	}
	if len(technologies) != 1 {
		t.Fatalf("technologies = %d, want 1", len(technologies))
	}
	if len(technologies[0].Tech) != 2 {
		t.Errorf("tech = %v, want [nginx thinkphp]", technologies[0].Tech)
	}
}

func TestParseSprayRecordSkipsInvalid(t *testing.T) {
	directories, technologies, err := ParseSprayRecord(sprayRecord{IsValid: false})
	if err != nil {
		t.Fatalf("ParseSprayRecord: %v", err)
	}
	if len(directories) != 0 || len(technologies) != 0 {
		t.Errorf("expected no results for invalid record")
	}
}

func TestParseSprayRecordRejectsNonHTTPURL(t *testing.T) {
	if _, _, err := ParseSprayRecord(sprayRecord{IsValid: true, UrlString: "ftp://example.com"}); err == nil {
		t.Fatal("expected error for non-http url")
	}
}

func argsString(args []string) string {
	out := ""
	for index, arg := range args {
		if index > 0 {
			out += " "
		}
		out += arg
	}
	return out
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
