// Package domain — 领域模型
// 对应架构文档 §5 数据模型
package domain

import "time"

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parent_id"`
	Depth     int       `json:"depth"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	OrgID        string     `json:"org_id"`
	MFAEnabled   bool       `json:"mfa_enabled"`
	Disabled     bool       `json:"disabled"`
	LastLogin    *time.Time `json:"last_login"`
	CreatedAt    time.Time  `json:"created_at"`
}
