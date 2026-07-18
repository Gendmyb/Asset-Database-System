// Package handler — Organization memory store (DEMO 模式)
// 生产模式使用 repository.OrgRepo (PG ltree)
package handler

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OrgStore — ltree 物化路径组织树 (内存实现, 仅 DEMO 模式)
type OrgStore struct {
	items map[string]*OrgNode
}

func NewOrgStore() *OrgStore {
	s := &OrgStore{items: make(map[string]*OrgNode)}
	root := &OrgNode{
		ID: uuid.New().String(), Name: "Demo Corp", Depth: 0,
		Path: "root.Demo_Corp", CreatedAt: time.Now(), UpdatedAt: time.Now(),
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
			return nil, fmt.Errorf("not found")
		}
		if parent.Depth >= 20 {
			return nil, fmt.Errorf("max depth (20) exceeded")
		}
		parentPath = parent.Path
		depth = parent.Depth + 1
	} else {
		parentPath = "root.Demo_Corp"
		depth = 1
	}

	sanitized := name
	for _, c := range []string{" ", "."} {
		newS := ""
		for _, r := range sanitized {
			if string(r) == c {
				newS += "_"
			} else {
				newS += string(r)
			}
		}
		sanitized = newS
	}

	org := &OrgNode{
		ID:        uuid.New().String(),
		Name:      name,
		ParentID:  parentID,
		Depth:     depth,
		Path:      parentPath + "." + sanitized,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.items[org.ID] = org
	return org, nil
}

func (s *OrgStore) Find(id string) *OrgNode {
	return s.items[id]
}

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
