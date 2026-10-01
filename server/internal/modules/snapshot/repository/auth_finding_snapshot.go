package repository

import (
	"context"

	snapshotdomain "github.com/yyhuni/lunafox/server/internal/modules/snapshot/domain"
	"github.com/yyhuni/lunafox/server/internal/pkg/dbtx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AuthFindingSnapshotRepository struct{ db *gorm.DB }

func NewAuthFindingSnapshotRepository(db *gorm.DB) *AuthFindingSnapshotRepository {
	return &AuthFindingSnapshotRepository{db: db}
}

type authFindingSnapshotRow struct {
	ID      int    `gorm:"column:id;primaryKey"`
	ScanID  int    `gorm:"column:scan_id"`
	URL     string `gorm:"column:url"`
	Service string `gorm:"column:service"`
	Kind    string `gorm:"column:kind"`
	Account string `gorm:"column:account"`
}

func (authFindingSnapshotRow) TableName() string { return "auth_finding_snapshot" }

func (repo *AuthFindingSnapshotRepository) BatchCreateContext(ctx context.Context, snapshots []snapshotdomain.AuthFindingSnapshot) (int64, error) {
	if len(snapshots) == 0 {
		return 0, nil
	}
	rows := make([]authFindingSnapshotRow, 0, len(snapshots))
	for _, item := range snapshots {
		rows = append(rows, authFindingSnapshotRow{ScanID: item.ScanID, URL: item.URL, Service: item.Service, Kind: item.Kind, Account: item.Account})
	}
	result := dbtx.Resolve(ctx, repo.db).WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 100)
	return result.RowsAffected, result.Error
}
