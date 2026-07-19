// Package service — 审批执行回调 (Wave 2 G7)
//
// 将审批通过事件桥接到原业务操作。每个 Executor 解码 approval_requests.payload
// 并调用对应 service 方法, 使审批通过后才真正执行领用/报废/维修。
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AssignmentApprovalExecutor 领用审批通过后执行领用
type AssignmentApprovalExecutor struct {
	svc      *AssignmentService
	userRepo *repository.UserRepo
}

func NewAssignmentApprovalExecutor(svc *AssignmentService, userRepo *repository.UserRepo) *AssignmentApprovalExecutor {
	return &AssignmentApprovalExecutor{svc: svc, userRepo: userRepo}
}

func (e *AssignmentApprovalExecutor) Execute(ctx context.Context, pool *pgxpool.Pool, req *repository.ApprovalRequestRow) (interface{}, error) {
	var p struct {
		AssignedTo string `json:"assigned_to"`
		Notes      string `json:"notes"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	// 执行前校验 assigned_to 用户存在且未软删除, 避免创建指向已删除用户的领用记录
	if e.userRepo != nil {
		exists, err := e.userRepo.ExistsByID(ctx, pool, p.AssignedTo)
		if err != nil {
			return nil, fmt.Errorf("check assigned_to user: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("领用目标用户不存在或已删除: %s", p.AssignedTo)
		}
	}
	assignmentID, err := e.svc.Assign(ctx, pool, req.ResourceID, req.OrgID, p.AssignedTo, req.RequesterID, p.Notes)
	if err != nil {
		return nil, err
	}
	return map[string]string{"assignment_id": assignmentID}, nil
}

// RetirementApprovalExecutor 报废审批通过后执行报废
type RetirementApprovalExecutor struct {
	svc *MaintenanceService
}

func NewRetirementApprovalExecutor(svc *MaintenanceService) *RetirementApprovalExecutor {
	return &RetirementApprovalExecutor{svc: svc}
}

func (e *RetirementApprovalExecutor) Execute(ctx context.Context, pool *pgxpool.Pool, req *repository.ApprovalRequestRow) (interface{}, error) {
	var p struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	if err := e.svc.RetireAsset(ctx, pool, req.OrgID, req.ResourceID, p.Reason, req.RequesterID); err != nil {
		return nil, err
	}
	return map[string]string{"asset_id": req.ResourceID, "status": "retired"}, nil
}

// MaintenanceApprovalExecutor 维修工单审批通过后创建工单
type MaintenanceApprovalExecutor struct {
	svc *MaintenanceService
}

func NewMaintenanceApprovalExecutor(svc *MaintenanceService) *MaintenanceApprovalExecutor {
	return &MaintenanceApprovalExecutor{svc: svc}
}

func (e *MaintenanceApprovalExecutor) Execute(ctx context.Context, pool *pgxpool.Pool, req *repository.ApprovalRequestRow) (interface{}, error) {
	var p struct {
		Category    string  `json:"category"`
		Title       string  `json:"title"`
		Description *string `json:"description"`
		Assignee    *string `json:"assignee"`
		Vendor      *string `json:"vendor"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	mo, err := e.svc.CreateOrder(ctx, pool, req.OrgID, CreateOrderInput{
		AssetID:     req.ResourceID,
		Category:    p.Category,
		Title:       p.Title,
		Description: p.Description,
		ReportedBy:  req.RequesterID,
		Assignee:    p.Assignee,
		Vendor:      p.Vendor,
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"order_id": mo.ID, "order_no": mo.OrderNo}, nil
}
