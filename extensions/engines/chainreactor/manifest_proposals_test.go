package chainreactor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yyhuni/lunafox/contracts/enginecontract/engineexecution"
	"github.com/yyhuni/lunafox/contracts/enginemanifest"
)

// These are authoring drafts, outside Engine discovery. Keep their scan-config
// surfaces valid while the corresponding Facades and Packages are being built.
func TestProposedEngineManifestsDecodeStrictly(t *testing.T) {
	for _, name := range []string{"gogo_scan", "spray_scan", "zombie_audit"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "proposals", "engine-manifests", name+".engine.json")
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			definition, err := enginemanifest.DecodeEngineDefinition(payload, path)
			if err != nil {
				t.Fatal(err)
			}
			if definition.EngineID != "engine.lunafox."+name || len(definition.Execution.ConfigSections) != 1 {
				t.Fatalf("unexpected proposed Engine definition: %+v", definition)
			}
		})
	}
}

func TestInstalledEngineProfilesHaveCompleteDefaults(t *testing.T) {
	for _, name := range []string{"gogo_scan", "spray_scan", "zombie_audit"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", name, "engine.json")
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			definition, err := enginemanifest.DecodeEngineDefinition(payload, path)
			if err != nil {
				t.Fatal(err)
			}
			defaults, err := engineexecution.NormalizeAndValidateConfig(map[string]any{}, definition.Execution)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engineexecution.ValidateCompleteConfig(defaults, definition.Execution); err != nil {
				t.Fatal(err)
			}
		})
	}
}
