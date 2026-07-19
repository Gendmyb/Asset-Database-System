// Package scheduler — repo 适配器
// 将 repository 层的扫描方法适配为 scheduler.Scanner 接口。
package scheduler

import (
	"context"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RepoScanner 基于 AssetRepo + AssignmentRepo 的扫描器实现
type RepoScanner struct {
	assetRepo      *repository.AssetRepo
	assignmentRepo *repository.AssignmentRepo
	pool           *pgxpool.Pool
}

// NewRepoScanner 构造 RepoScanner
func NewRepoScanner(assetRepo *repository.AssetRepo, assignmentRepo *repository.AssignmentRepo, pool *pgxpool.Pool) *RepoScanner {
	return &RepoScanner{assetRepo: assetRepo, assignmentRepo: assignmentRepo, pool: pool}
}

// ScanWarrantyExpiring 扫描质保到期
func (s *RepoScanner) ScanWarrantyExpiring(ctx context.Context, days int) ([]WarrantyRow, error) {
	rows, err := s.assetRepo.ScanWarrantyExpiring(ctx, s.pool, days)
	if err != nil {
		return nil, err
	}
	out := make([]WarrantyRow, len(rows))
	for i, r := range rows {
		out[i] = WarrantyRow{
			AssetID:       r.AssetID,
			AssetTag:      r.AssetTag,
			Name:          r.Name,
			OrgID:         r.OrgID,
			WarrantyUntil: r.WarrantyUntil,
			Expired:       r.Expired,
		}
	}
	return out, nil
}

// ScanOverdueAssignments 扫描逾期领用
func (s *RepoScanner) ScanOverdueAssignments(ctx context.Context) ([]OverdueRow, error) {
	rows, err := s.assignmentRepo.ScanOverdueAssignments(ctx, s.pool)
	if err != nil {
		return nil, err
	}
	out := make([]OverdueRow, len(rows))
	for i, r := range rows {
		out[i] = OverdueRow{
			AssignmentID: r.AssignmentID,
			AssetID:      r.AssetID,
			AssetTag:     r.AssetTag,
			AssetName:    r.AssetName,
			OrgID:        r.OrgID,
			AssignedTo:   r.AssignedTo,
			DueDate:      r.DueDate,
		}
	}
	return out, nil
}
