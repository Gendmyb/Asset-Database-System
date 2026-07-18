// Package handler — Location memory store (DEMO 模式)
// 生产模式使用 repository.LocationRepo (PG)
package handler

// LocationStore 内存存储 (仅 DEMO 模式)
type LocationStore struct {
	items []*Location
}

func NewLocationStore() *LocationStore {
	return &LocationStore{items: make([]*Location, 0)}
}

func (s *LocationStore) List(orgID string) []*Location {
	var result []*Location
	for _, l := range s.items {
		if l.OrgID == orgID {
			result = append(result, l)
		}
	}
	if result == nil {
		result = []*Location{}
	}
	return result
}

func (s *LocationStore) Add(l *Location) {
	s.items = append(s.items, l)
}
