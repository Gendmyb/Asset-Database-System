// Package handler — 资产 API 集成测试
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ============================================================
// MockAssetRepository — 资产仓库模拟
// ============================================================

type mockAssetRepo struct {
	assets  map[string]*Asset
	history map[string][]AuditLog
	createErr error
	updateErr error
	deleteErr error
	getByIDErr error
}

func newMockAssetRepo() *mockAssetRepo {
	return &mockAssetRepo{
		assets:  make(map[string]*Asset),
		history: make(map[string][]AuditLog),
	}
}

func (m *mockAssetRepo) List(orgID string, search string, typeID string, status string, cursor string, limit int) ([]Asset, string, bool, error) {
	var result []Asset
	for _, a := range m.assets {
		if a.OrgID == orgID {
			result = append(result, *a)
		}
	}
	return result, "", len(result) > limit, nil
}

func (m *mockAssetRepo) GetByID(id string) (*Asset, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	a, ok := m.assets[id]
	if !ok {
		return nil, errors.New("not found")
	}
	if a.DeletedAt != nil {
		return nil, errors.New("not found")
	}
	return a, nil
}

func (m *mockAssetRepo) Create(asset *Asset) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.assets[asset.ID] = asset
	return nil
}

func (m *mockAssetRepo) Update(id string, updates map[string]interface{}, version int) (*Asset, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	a, ok := m.assets[id]
	if !ok {
		return nil, errors.New("not found")
	}
	if a.Version != version {
		return nil, fmt.Errorf("version conflict")
	}
	// Apply updates
	if v, ok := updates["name"]; ok {
		a.Name = v.(string)
	}
	if v, ok := updates["lifecycle_state"]; ok {
		a.LifecycleState = v.(string)
	}
	if v, ok := updates["status"]; ok {
		a.Status = v.(string)
	}
	a.Version++
	a.UpdatedAt = time.Now()
	return a, nil
}

func (m *mockAssetRepo) SoftDelete(id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	a, ok := m.assets[id]
	if !ok || a.DeletedAt != nil {
		return errors.New("asset not found")
	}
	now := time.Now()
	a.DeletedAt = &now
	return nil
}

func (m *mockAssetRepo) GetHistory(assetID string, limit int) ([]AuditLog, error) {
	return m.history[assetID], nil
}

// ============================================================
// setupTestRouter — 创建测试 Gin 引擎
// ============================================================

func setupTestRouter(repo AssetRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// 注入 org_id 中间件
	r.Use(func(c *gin.Context) {
		c.Set("org_id", "test-org-id")
		c.Next()
	})

	handler := NewAssetHandler(repo)
	r.POST("/api/v1/assets", handler.CreateAsset)
	r.GET("/api/v1/assets/:id", handler.GetAsset)
	r.PUT("/api/v1/assets/:id", handler.UpdateAsset)
	r.DELETE("/api/v1/assets/:id", handler.DeleteAsset)
	r.GET("/api/v1/assets/:id/history", handler.GetHistory)
	r.GET("/api/v1/assets", handler.ListAssets)

	return r
}

// ============================================================
// TestCreateAsset — 创建资产验证
// ============================================================

func TestCreateAsset(t *testing.T) {
	repo := newMockAssetRepo()
	router := setupTestRouter(repo)

	body := map[string]interface{}{
		"asset_tag":     "ASSET-001",
		"name":          "MacBook Pro 16",
		"type_id":       "laptop",
		"lifecycle_state": "procurement",
		"status":        "available",
	}

	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/v1/assets", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data Asset `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// 验证创建结果
	if resp.Data.AssetTag != "ASSET-001" {
		t.Errorf("asset_tag: want ASSET-001, got %s", resp.Data.AssetTag)
	}
	if resp.Data.Name != "MacBook Pro 16" {
		t.Errorf("name: want MacBook Pro 16, got %s", resp.Data.Name)
	}
	if resp.Data.OrgID != "test-org-id" {
		t.Errorf("org_id: want test-org-id, got %s", resp.Data.OrgID)
	}
	if resp.Data.Version != 1 {
		t.Errorf("version: want 1, got %d", resp.Data.Version)
	}
	if resp.Data.LifecycleState != "procurement" {
		t.Errorf("lifecycle_state: want procurement, got %s", resp.Data.LifecycleState)
	}
	if resp.Data.ID == "" {
		t.Error("id should not be empty")
	}

	// 验证 repo 中确实存储了
	stored, err := repo.GetByID(resp.Data.ID)
	if err != nil {
		t.Fatalf("asset not found in repo: %v", err)
	}
	if stored.AssetTag != "ASSET-001" {
		t.Errorf("stored asset_tag mismatch")
	}
}

// ============================================================
// TestUpdateAssetOptimisticLock — 乐观锁冲突测试
// ============================================================

func TestUpdateAssetOptimisticLock(t *testing.T) {
	repo := newMockAssetRepo()
	router := setupTestRouter(repo)

	// 先创建一个资产
	asset := &Asset{
		ID:             "test-asset-id",
		AssetTag:       "ASSET-002",
		Name:           "Original Name",
		TypeID:         "laptop",
		OrgID:          "test-org-id",
		LifecycleState: "procurement",
		Status:         "available",
		Version:        1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	repo.assets[asset.ID] = asset

	// 使用正确版本更新 — 应成功
	updates := map[string]string{"name": "Updated Name"}
	data, _ := json.Marshal(updates)
	req, _ := http.NewRequest("PUT", "/api/v1/assets/test-asset-id", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", "1")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data Asset `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Name != "Updated Name" {
		t.Errorf("name: want Updated Name, got %s", resp.Data.Name)
	}
	if resp.Data.Version != 2 {
		t.Errorf("version should be 2 after update, got %d", resp.Data.Version)
	}

	// 使用过期版本 — 应返回 409 Conflict
	updates2 := map[string]string{"name": "Concurrent Update"}
	data2, _ := json.Marshal(updates2)
	req2, _ := http.NewRequest("PUT", "/api/v1/assets/test-asset-id", bytes.NewBuffer(data2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("If-Match", "1") // 过期版本

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for stale version, got %d: %s", w2.Code, w2.Body.String())
	}

	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(w2.Body.Bytes(), &errResp)
	if errResp.Error.Code != "VERSION_CONFLICT" {
		t.Errorf("expected VERSION_CONFLICT, got %s", errResp.Error.Code)
	}
}

// ============================================================
// TestUpdateAssetMissingIfMatch — 缺少 If-Match 头
// ============================================================

func TestUpdateAssetMissingIfMatch(t *testing.T) {
	repo := newMockAssetRepo()
	router := setupTestRouter(repo)

	asset := &Asset{
		ID: "test-id", AssetTag: "TAG", Name: "N", TypeID: "t",
		OrgID: "test-org-id", LifecycleState: "procurement", Status: "available",
		Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.assets[asset.ID] = asset

	data, _ := json.Marshal(map[string]string{"name": "New"})
	req, _ := http.NewRequest("PUT", "/api/v1/assets/test-id", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	// 不设置 If-Match

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428 Precondition Required, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================
// TestDeleteAssetSoftDelete — 软删除后查询返回404
// ============================================================

func TestDeleteAssetSoftDelete(t *testing.T) {
	repo := newMockAssetRepo()
	router := setupTestRouter(repo)

	// 先创建一个资产
	asset := &Asset{
		ID:             "soft-delete-test-id",
		AssetTag:       "ASSET-003",
		Name:           "To Be Deleted",
		TypeID:         "laptop",
		OrgID:          "test-org-id",
		LifecycleState: "retirement",
		Status:         "retired",
		Version:        1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	repo.assets[asset.ID] = asset

	// 删除前验证可查询
	reqGet, _ := http.NewRequest("GET", "/api/v1/assets/soft-delete-test-id", nil)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 before delete, got %d: %s", wGet.Code, wGet.Body.String())
	}

	// 执行软删除
	reqDel, _ := http.NewRequest("DELETE", "/api/v1/assets/soft-delete-test-id", nil)
	wDel := httptest.NewRecorder()
	router.ServeHTTP(wDel, reqDel)

	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d: %s", wDel.Code, wDel.Body.String())
	}

	// 删除后验证返回 404
	reqGet2, _ := http.NewRequest("GET", "/api/v1/assets/soft-delete-test-id", nil)
	wGet2 := httptest.NewRecorder()
	router.ServeHTTP(wGet2, reqGet2)

	if wGet2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after soft delete, got %d: %s", wGet2.Code, wGet2.Body.String())
	}

	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(wGet2.Body.Bytes(), &errResp)
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND code, got %s", errResp.Error.Code)
	}

	// 验证 repo 中的 deleted_at 已设置
	stored, _ := repo.assets["soft-delete-test-id"]
	if stored.DeletedAt == nil {
		t.Error("expected deleted_at to be set after soft delete")
	}

	// 再次删除应返回 404
	reqDel2, _ := http.NewRequest("DELETE", "/api/v1/assets/soft-delete-test-id", nil)
	wDel2 := httptest.NewRecorder()
	router.ServeHTTP(wDel2, reqDel2)
	if wDel2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on second delete, got %d", wDel2.Code)
	}
}
