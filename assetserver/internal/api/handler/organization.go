// Package handler — Organization CRUD (Phase 5, ltree 物化路径)
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OrgNode 组织节点
type OrgNode struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ParentID  *string    `json:"parent_id"`
	Depth     int        `json:"depth"`
	Path      string     `json:"path"` // ltree 物化路径
	Children  []*OrgNode `json:"children,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// OrgHandler 组织管理
type OrgHandler struct {
	store *OrgStore
}

func NewOrgHandler() *OrgHandler {
	return &OrgHandler{store: NewOrgStore()}
}

// List GET /api/v1/organizations
func (h *OrgHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": h.store.Tree()})
}

// Create POST /api/v1/organizations
func (h *OrgHandler) Create(c *gin.Context) {
	var input struct {
		Name     string  `json:"name" binding:"required"`
		ParentID *string `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	org, err := h.store.Add(input.Name, input.ParentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": org})
}

// Get GET /api/v1/organizations/:id
func (h *OrgHandler) Get(c *gin.Context) {
	org := h.store.Find(c.Param("id"))
	if org == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": org})
}

// Subtree GET /api/v1/organizations/:id/subtree (ltree 子组织查询)
func (h *OrgHandler) Subtree(c *gin.Context) {
	org := h.store.Find(c.Param("id"))
	if org == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	subtree := h.store.Subtree(org.Path)
	c.JSON(http.StatusOK, gin.H{"data": subtree})
}

// ===================================================================
// OrgStore — ltree 物化路径组织树 (内存实现)
// 生产环境: PostgreSQL ltree 扩展 + GIST 索引
// 对应架构文档 §7.2 多租户隔离
// ===================================================================

type OrgStore struct {
	items map[string]*OrgNode
}

func NewOrgStore() *OrgStore {
	s := &OrgStore{items: make(map[string]*OrgNode)}
	// 种子: 根组织
	root := &OrgNode{
		ID: uuid.New().String(), Name: "Demo Corp", Depth: 0,
		Path: "root", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.items[root.ID] = root
	return s
}

func (s *OrgStore) Add(name string, parentID *string) (*OrgNode, error) {
	var parentPath string
	var depth int
	if parentID != nil {
		parent, ok := s.items[*parentID]
		if !ok {
			return nil, ErrNotFound
		}
		if parent.Depth >= 20 {
			return nil, ErrMaxDepth
		}
		parentPath = parent.Path
		depth = parent.Depth + 1
	} else {
		parentPath = "root"
		depth = 1
	}

	org := &OrgNode{
		ID:        uuid.New().String(),
		Name:      name,
		ParentID:  parentID,
		Depth:     depth,
		Path:      parentPath + "." + name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.items[org.ID] = org
	return org, nil
}

func (s *OrgStore) Find(id string) *OrgNode {
	return s.items[id]
}

// Tree 返回组织树
func (s *OrgStore) Tree() []*OrgNode {
	var roots []*OrgNode
	children := make(map[string][]*OrgNode)
	for _, org := range s.items {
		if org.ParentID == nil {
			roots = append(roots, org)
		} else {
			children[*org.ParentID] = append(children[*org.ParentID], org)
		}
	}
	for _, org := range s.items {
		org.Children = children[org.ID]
	}
	return roots
}

// Subtree 查询 path 前缀的子孙组织 (对应 ltree: path <@ parent_path)
func (s *OrgStore) Subtree(path string) []*OrgNode {
	var result []*OrgNode
	for _, org := range s.items {
		if len(org.Path) >= len(path) && org.Path[:len(path)] == path {
			result = append(result, org)
		}
	}
	if result == nil {
		result = []*OrgNode{}
	}
	return result
}

var (
	ErrNotFound  = &orgError{"organization not found"}
	ErrMaxDepth  = &orgError{"max depth (20) exceeded"}
)

type orgError struct{ msg string }
func (e *orgError) Error() string { return e.msg }
