package handler

import "testing"

func TestOrgTree(t *testing.T) {
	s := NewOrgStore()

	// 根节点
	if len(s.Tree()) != 1 {
		t.Fatal("expected 1 root")
	}

	// 添加子组织
	root := s.Tree()[0]
	eng, _ := s.Add("Engineering", &root.ID)
	s.Add("Backend", &eng.ID)
	s.Add("Frontend", &eng.ID)
	s.Add("Marketing", &root.ID)

	// 深度验证
	found := s.Find(eng.ID)
	if found.Depth != 1 {
		t.Errorf("Engineering depth: want 1, got %d", found.Depth)
	}

	// 子树查询 (ltree <@)
	subtree := s.Subtree(root.Path)
	if len(subtree) < 4 {
		t.Errorf("subtree too small: %d", len(subtree))
	}

	// 深度限制
	var parentID = &eng.ID
	for i := 0; i < 21; i++ {
		child, err := s.Add("deep", parentID)
		if err != nil {
			if i >= 19 {
				break // expected at depth > 20
			}
			t.Fatalf("unexpected error at depth %d: %v", i+2, err)
		}
		id := child.ID
		parentID = &id
	}
}
