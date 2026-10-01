package application

import (
	"context"
	"errors"

	"github.com/yyhuni/lunafox/contracts/results"
	snapshotdomain "github.com/yyhuni/lunafox/server/internal/modules/snapshot/domain"
	"github.com/yyhuni/lunafox/server/internal/pkg/dberrors"
)

type AuthFindingSnapshotCommandStore interface {
	BatchCreateContext(context.Context, []snapshotdomain.AuthFindingSnapshot) (int64, error)
}

type AuthFindingSnapshotService struct {
	store      AuthFindingSnapshotCommandStore
	scanLookup SnapshotApplicationScanRefLookup
}

func NewAuthFindingSnapshotService(store AuthFindingSnapshotCommandStore, scanLookup SnapshotApplicationScanRefLookup) *AuthFindingSnapshotService {
	return &AuthFindingSnapshotService{store: store, scanLookup: scanLookup}
}

func (service *AuthFindingSnapshotService) SaveResultBatchContext(ctx context.Context, scanID, targetID int, items []results.AuthFinding) (MaterializationSummary, error) {
	summary := MaterializationSummary{ReceivedItems: len(items)}
	if ctx == nil {
		return summary, errors.New("auth finding context is required")
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	if service == nil || service.store == nil || service.scanLookup == nil {
		return summary, errors.New("auth finding dependencies are required")
	}
	scan, err := service.scanLookup.GetScanRefByIDContext(ctx, scanID)
	if dberrors.IsRecordNotFound(err) {
		return summary, ErrSnapshotScanNotFound
	}
	if err != nil {
		return summary, err
	}
	if scan.TargetID != targetID {
		return summary, ErrSnapshotTargetMismatch
	}
	target, err := service.scanLookup.GetTargetRefByScanIDContext(ctx, scanID)
	if dberrors.IsRecordNotFound(err) {
		return summary, ErrSnapshotScanNotFound
	}
	if err != nil {
		return summary, err
	}
	seen := make(map[results.AuthFinding]struct{}, len(items))
	snapshots := make([]snapshotdomain.AuthFindingSnapshot, 0, len(items))
	for _, item := range items {
		if err := results.Validate(results.ResultKindSecurityAuthFinding, item); err != nil {
			summary.InvalidItems++
			continue
		}
		if !snapshotdomain.IsURLMatchTarget(item.URL, *target) {
			summary.ScopeFilteredItems++
			continue
		}
		if _, exists := seen[item]; exists {
			summary.DuplicateItems++
			continue
		}
		seen[item] = struct{}{}
		snapshots = append(snapshots, snapshotdomain.AuthFindingSnapshot{ScanID: scanID, URL: item.URL, Service: item.Service, Kind: item.Kind, Account: item.Account})
	}
	if len(snapshots) == 0 {
		return summary, nil
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	summary.SnapshotCount, err = service.store.BatchCreateContext(ctx, snapshots)
	if err == nil {
		summary.DuplicateItems += len(snapshots) - int(summary.SnapshotCount)
	}
	return summary, err
}
