// Package api — LDAP 适配器
// 将 internal/auth/ldap.AuthService 适配为 service.LDAPAuthenticator 接口,
// 避免在 ldap 包中反向依赖 service 包 (保持依赖方向: api -> service, api -> ldap)。
package api

import (
	"context"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/auth/ldap"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
)

// ldapAdapter 桥接 ldap.AuthService -> service.LDAPAuthenticator
type ldapAdapter struct {
	inner *ldap.AuthService
}

func newLDAPAdapter(inner *ldap.AuthService) service.LDAPAuthenticator {
	return &ldapAdapter{inner: inner}
}

func (a *ldapAdapter) Authenticate(ctx context.Context, username, password string) (*service.LDAPAuthResult, error) {
	r, err := a.inner.Authenticate(ctx, username, password)
	if err != nil || r == nil {
		return nil, err
	}
	return &service.LDAPAuthResult{
		Valid:       r.Valid,
		Username:    r.Username,
		DisplayName: r.DisplayName,
		Email:       r.Email,
		DN:          r.DN,
	}, nil
}

func (a *ldapAdapter) EnsureUserRow(ctx context.Context, r *service.LDAPAuthResult, defaultOrgID string) (string, string, string, error) {
	if r == nil {
		return "", "", "", nil
	}
	return a.inner.EnsureUserRow(ctx, &ldap.AuthenticateResult{
		Valid:       r.Valid,
		Username:    r.Username,
		DisplayName: r.DisplayName,
		Email:       r.Email,
		DN:          r.DN,
	}, defaultOrgID)
}
