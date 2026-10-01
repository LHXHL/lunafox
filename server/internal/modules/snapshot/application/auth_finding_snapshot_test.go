package application

import (
	"context"
	"testing"

	"github.com/yyhuni/lunafox/contracts/results"
	snapshotdomain "github.com/yyhuni/lunafox/server/internal/modules/snapshot/domain"
)

type authFindingSnapshotStoreStub struct {
	items []snapshotdomain.AuthFindingSnapshot
	count int64
}

func (stub *authFindingSnapshotStoreStub) BatchCreateContext(_ context.Context, items []snapshotdomain.AuthFindingSnapshot) (int64, error) {
	stub.items = append([]snapshotdomain.AuthFindingSnapshot(nil), items...)
	return stub.count, nil
}

func TestAuthFindingMaterializationChecksScopeAndDeduplicates(t *testing.T) {
	store := &authFindingSnapshotStoreStub{count: 1}
	lookup := &snapshotScanLookupStub{scan: &snapshotdomain.ScanRef{ID: 2, TargetID: 8}, target: &snapshotdomain.ScanTargetRef{ID: 8, Name: "example.com", Type: "domain"}}
	service := NewAuthFindingSnapshotService(store, lookup)
	valid := results.AuthFinding{URL: "https://auth.example.com/", Service: "https", Kind: "valid_credential", Account: "test"}
	summary, err := service.SaveResultBatchContext(context.Background(), 2, 8, []results.AuthFinding{
		valid, valid,
		{URL: "https://outside.test/", Service: "https", Kind: "anonymous_access"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.ReceivedItems != 3 || summary.SnapshotCount != 1 || summary.DuplicateItems != 1 || summary.ScopeFilteredItems != 1 || len(store.items) != 1 || store.items[0].Account != "test" {
		t.Fatalf("unexpected materialization: summary=%+v items=%+v", summary, store.items)
	}
}

func TestAuthFindingMaterializationCountsPreviouslyStoredDuplicate(t *testing.T) {
	store := &authFindingSnapshotStoreStub{}
	lookup := &snapshotScanLookupStub{scan: &snapshotdomain.ScanRef{ID: 2, TargetID: 8}, target: &snapshotdomain.ScanTargetRef{ID: 8, Name: "example.com", Type: "domain"}}
	service := NewAuthFindingSnapshotService(store, lookup)
	summary, err := service.SaveResultBatchContext(context.Background(), 2, 8, []results.AuthFinding{{URL: "https://example.com/", Service: "https", Kind: "anonymous_access"}})
	if err != nil || summary.DuplicateItems != 1 || summary.SnapshotCount != 0 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
}
