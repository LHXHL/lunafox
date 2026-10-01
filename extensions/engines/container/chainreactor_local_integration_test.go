package containercontract

import (
	"context"
	"os"
	"testing"
	"time"
)

	// This focused development test uses the ordinary Engine API v2 fixture
	// against immutable local images. CI does not need a local Registry.
func TestChainreactorLocalImageExecution(t *testing.T) {
	references := map[string]string{
		"gogo_scan":  os.Getenv("CHAINREACTOR_GOGO_IMAGE"),
		"spray_scan": os.Getenv("CHAINREACTOR_SPRAY_IMAGE"),
		"zombie_audit": os.Getenv("CHAINREACTOR_ZOMBIE_IMAGE"),
	}
	if references["gogo_scan"] == "" && references["spray_scan"] == "" && references["zombie_audit"] == "" {
		t.Skip("set at least one CHAINREACTOR_*_IMAGE to an immutable local image ref")
	}
	inventory, err := loadToolInventory(testRepoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	r := &runner{
		options: Options{RepoRoot: testRepoRoot(t), Platform: "linux/arm64", DockerCommand: "docker", DaemonVisibleRoot: "/tmp"},
		exec:    systemCommandExecutor{},
	}
	if err := r.prepareUDSProxy(ctx); err != nil {
		t.Fatal(err)
	}
	defer r.cleanupUDSProxy()
	for _, engine := range inventory.Engines {
		reference := references[engine.Directory]
		if reference == "" {
			continue
		}
		t.Run(engine.Directory, func(t *testing.T) {
			if _, err := parseRuntimeImageReference(reference, engine.EngineID); err != nil {
				t.Fatal(err)
			}
			if _, err := r.verifyImage(ctx, selectedImage{engine: engine, reference: reference}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
