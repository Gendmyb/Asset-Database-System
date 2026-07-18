// Package handler — Organization CRUD (PG ltree 物化路径)
// Phase B: 生产模式走 PG repo
package handler

import (
	"net/http"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
)

// OrgNode 组织节点 (API 响应)
type OrgNode struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ParentID  *string    `json:"parent_id"`
	Depth     int        `json:"depth"`
	Path      string     `json:"path"`
	Children  []*OrgNode `json:"children,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// OrgHandler 组织管理
type OrgHandler struct {
	repo *repository.OrgRepo
	pool repository.DBTX
}

func NewOrgHandler(repo *repository.OrgRepo, pool repository.DBTX) *OrgHandler {
	return &OrgHandler{repo: repo, pool: pool}
}

// List GET /api/v1/organizations (树结构)
func (h *OrgHandler) List(c *gin.Context) {
	rows, err := h.repo.ListOrgs(c.Request.Context(), h.pool)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tree := buildOrgTree(rows)
	c.JSON(http.StatusOK, gin.H{"data": tree})
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

	org, err := h.repo.CreateOrg(c.Request.Context(), h.pool, repository.CreateOrgInput{
		Name:     input.Name,
		ParentID: input.ParentID,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": rowToOrgNode(org)})
}

// Get GET /api/v1/organizations/:id
func (h *OrgHandler) Get(c *gin.Context) {
	org, err := h.repo.GetOrg(c.Request.Context(), h.pool, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rowToOrgNode(org)})
}

// Subtree GET /api/v1/organizations/:id/subtree (ltree 子组织查询)
func (h *OrgHandler) Subtree(c *gin.Context) {
	org, err := h.repo.GetOrg(c.Request.Context(), h.pool, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	rows, err := h.repo.Subtree(c.Request.Context(), h.pool, org.Path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	nodes := make([]*OrgNode, len(rows))
	for i, r := range rows {
		n := rowToOrgNode(&r)
		nodes[i] = n
	}
	c.JSON(http.StatusOK, gin.H{"data": nodes})
}

func rowToOrgNode(r *repository.OrgRow) *OrgNode {
	return &OrgNode{
		ID:        r.ID,
		Name:      r.Name,
		ParentID:  r.ParentID,
		Depth:     r.Depth,
		Path:      r.Path,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

// buildOrgTree 将扁平列表构建为树结构
func buildOrgTree(rows []repository.OrgRow) []*OrgNode {
	nodes := make(map[string]*OrgNode)
	var roots []*OrgNode
	children := make(map[string][]*OrgNode)

	for i := range rows {
		n := rowToOrgNode(&rows[i])
		nodes[n.ID] = n
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n)
		} else {
			roots = append(roots, n)
		}
	}
	for id, kids := range children {
		if n, ok := nodes[id]; ok {
			n.Children = kids
		}
	}
	if roots == nil {
		roots = []*OrgNode{}
	}
	return roots
}
