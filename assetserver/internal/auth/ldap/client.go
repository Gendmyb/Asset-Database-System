// Package ldap — AD/LDAP 集成 (同步 + bind 登录)
// Wave 1 G1: 与本系统本地认证并行, 配置未启用时纯本地运行。
//
// 安全策略:
//   - 服务账号 / 用户密码绝不入日志与审计 (audit 只记 username 与结果)。
//   - 登录回退顺序: 本地用户优先 (source='local' 且密码匹配) → LDAP bind 兜底。
//     保留本地 admin 在 AD 故障/未配置时仍可登录系统。
package ldap

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/go-ldap/ldap/v3"
)

// DirectoryClient 抽象 LDAP 目录访问, 便于单测 mock。
type DirectoryClient interface {
	// SearchUsers 返回所有 (匹配 filter 的) 用户条目 (供 sync 全量同步使用)
	SearchUsers(ctx context.Context) ([]DirUser, error)
	// SearchOne 用 cfg.UserFilter 精确查询单个用户 (登录路径使用, 不拉全量)
	SearchOne(ctx context.Context, username string) (*DirUser, error)
	// Bind 校验用户凭据 (userDN + password)
	Bind(ctx context.Context, userDN, password string) error
	// Close 释放底层连接
	Close() error
}

// DirUser 目录中的用户条目 (从 AD 拉取后映射为本系统字段)
type DirUser struct {
	Username    string // sAMAccountName (登录名)
	DisplayName string // displayName
	Email       string // mail
	DN          string // 完整 distinguishedName
	Department  string // department / 部门名
}

// adClient go-ldap 实现
type adClient struct {
	cfg config.LDAPConfig
}

// NewClient 构造 LDAP 客户端 (未调用 Connect, 每次方法内拨号)
func NewClient(cfg config.LDAPConfig) DirectoryClient {
	return &adClient{cfg: cfg}
}

// dial 建立新连接并执行服务账号 bind
// 不复用连接: 简化并发与超时控制; 同步/登录频次低, 开销可接受。
func (c *adClient) dial(ctx context.Context) (*ldap.Conn, error) {
	if !c.cfg.Enable {
		return nil, fmt.Errorf("ldap disabled")
	}
	addr := fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)
	var (
		conn *ldap.Conn
		err  error
	)
	switch {
	case c.cfg.UseSSL:
		conn, err = ldap.DialURL("ldaps://"+addr,
			ldap.DialWithTLSConfig(&tls.Config{ServerName: c.cfg.Host}))
	default:
		conn, err = ldap.DialURL("ldap://" + addr)
	}
	if err != nil {
		return nil, fmt.Errorf("ldap dial: %w", err)
	}
	if c.cfg.UseTLS && !c.cfg.UseSSL {
		if err := conn.StartTLS(&tls.Config{ServerName: c.cfg.Host}); err != nil {
			conn.Close()
			return nil, fmt.Errorf("ldap starttls: %w", err)
		}
	}
	// 服务账号 bind (凭据绝不入日志)
	if err := conn.Bind(c.cfg.BindDN, c.cfg.BindPassword); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ldap service bind: %w", err)
	}
	return conn, nil
}

// SearchUsers 遍历 BaseDN 下所有匹配 objectClass=user 的条目
func (c *adClient) SearchUsers(ctx context.Context) ([]DirUser, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := ldap.NewSearchRequest(
		c.cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=user)",
		[]string{
			c.cfg.AttrUsername, c.cfg.AttrDisplayName, c.cfg.AttrEmail,
			c.cfg.AttrDN, c.cfg.AttrOrg,
		},
		nil,
	)
	res, err := conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("ldap search: %w", err)
	}
	users := make([]DirUser, 0, len(res.Entries))
	for _, e := range res.Entries {
		u := DirUser{
			Username:    e.GetAttributeValue(c.cfg.AttrUsername),
			DisplayName: e.GetAttributeValue(c.cfg.AttrDisplayName),
			Email:       e.GetAttributeValue(c.cfg.AttrEmail),
			DN:          e.GetAttributeValue(c.cfg.AttrDN),
			Department:  e.GetAttributeValue(c.cfg.AttrOrg),
		}
		if u.DN == "" {
			u.DN = e.DN
		}
		if u.Username == "" {
			// 无登录名条目跳过 (服务账号/联系人等)
			continue
		}
		users = append(users, u)
	}
	slog.Info("ldap sync: fetched users", "count", len(users))
	return users, nil
}

// SearchOne 用 cfg.UserFilter 精确查询 username 对应的单个用户条目。
// 登录路径使用, 避免拉全量目录; username 经 ldap.EscapeFilter 转义防注入。
// UserFilter 中的 %s 占位符被替换为转义后的 username; 若配置不含 %s 则追加默认过滤。
// 找不到返回 (nil, nil); 配置/目录错误返回 error。
func (c *adClient) SearchOne(ctx context.Context, username string) (*DirUser, error) {
	if username == "" {
		return nil, nil
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	filter := c.cfg.UserFilter
	if filter == "" {
		filter = "(&(objectClass=user)(sAMAccountName=%s))"
	}
	escaped := ldap.EscapeFilter(username)
	if strings.Contains(filter, "%s") {
		filter = strings.ReplaceAll(filter, "%s", escaped)
	} else {
		// 配置未提供占位符: 追加 sAMAccountName 过滤兜底
		filter = "(&" + filter + "(sAMAccountName=" + escaped + "))"
	}

	req := ldap.NewSearchRequest(
		c.cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		[]string{
			c.cfg.AttrUsername, c.cfg.AttrDisplayName, c.cfg.AttrEmail,
			c.cfg.AttrDN, c.cfg.AttrOrg,
		},
		nil,
	)
	res, err := conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("ldap search one: %w", err)
	}
	if len(res.Entries) == 0 {
		return nil, nil
	}
	e := res.Entries[0]
	u := DirUser{
		Username:    e.GetAttributeValue(c.cfg.AttrUsername),
		DisplayName: e.GetAttributeValue(c.cfg.AttrDisplayName),
		Email:       e.GetAttributeValue(c.cfg.AttrEmail),
		DN:          e.GetAttributeValue(c.cfg.AttrDN),
		Department:  e.GetAttributeValue(c.cfg.AttrOrg),
	}
	if u.DN == "" {
		u.DN = e.DN
	}
	if u.Username == "" {
		// 条目无登录名: 视为未找到 (避免 bind 阶段用空 DN)
		return nil, nil
	}
	return &u, nil
}

// Bind 用用户提供的凭据做 bind 校验
// 成功 = 凭据有效; 失败 = 拒绝。绝不打印 password。
func (c *adClient) Bind(ctx context.Context, userDN, password string) error {
	if userDN == "" || password == "" {
		return fmt.Errorf("empty credentials")
	}
	addr := fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)
	var (
		conn *ldap.Conn
		err  error
	)
	if c.cfg.UseSSL {
		conn, err = ldap.DialURL("ldaps://"+addr,
			ldap.DialWithTLSConfig(&tls.Config{ServerName: c.cfg.Host}))
	} else {
		conn, err = ldap.DialURL("ldap://" + addr)
	}
	if err != nil {
		return fmt.Errorf("ldap dial: %w", err)
	}
	defer conn.Close()
	if c.cfg.UseTLS && !c.cfg.UseSSL {
		if err := conn.StartTLS(&tls.Config{ServerName: c.cfg.Host}); err != nil {
			return fmt.Errorf("ldap starttls: %w", err)
		}
	}
	if err := conn.Bind(userDN, password); err != nil {
		return fmt.Errorf("ldap bind failed: %w", err)
	}
	return nil
}

func (c *adClient) Close() error { return nil }
