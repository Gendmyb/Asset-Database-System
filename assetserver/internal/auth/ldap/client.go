// Package ldap — AD/LDAP 集成 (同步 + bind 登录)
// Wave 1 G1: 与本系统本地认证并行, 配置未启用时纯本地运行。
// Wave 3 T1: 加固 — 分页、按组过滤、memberOf/userAccountControl、防注入。
//
// 安全策略:
//   - 服务账号 / 用户密码绝不入日志与审计 (audit 只记 username 与结果)。
//   - 登录回退顺序: 本地用户优先 (source='local' 且密码匹配) → LDAP bind 兜底。
//     保留本地 admin 在 AD 故障/未配置时仍可登录系统。
//   - LDAP filter 中所有用户输入经 ldap.EscapeFilter 转义, 防注入。
//   - TLS 配置不变: StartTLS / LDAPS 二选一, ServerName 校验。
package ldap

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/go-ldap/ldap/v3"
)

// DirectoryClient 抽象 LDAP 目录访问, 便于单测 mock。
type DirectoryClient interface {
	// SearchUsers 按启用的组映射过滤用户 (供 sync 全量同步使用)。
	// groupDNs: 启用的安全组 DN 列表。为空时回退到 (objectClass=user) 全量拉取 (兼容旧行为)。
	// 返回条目含 memberOf 和 userAccountControl。
	SearchUsers(ctx context.Context, groupDNs []string) ([]DirUser, error)
	// SearchOne 用 cfg.UserFilter 精确查询单个用户 (登录路径使用, 不拉全量)
	// 返回条目含 memberOf 和 userAccountControl
	SearchOne(ctx context.Context, username string) (*DirUser, error)
	// Bind 校验用户凭据 (userDN + password)
	Bind(ctx context.Context, userDN, password string) error
	// Close 释放底层连接
	Close() error
}

// DirUser 目录中的用户条目 (从 AD 拉取后映射为本系统字段)
type DirUser struct {
	Username          string   // sAMAccountName (登录名)
	DisplayName       string   // displayName
	Email             string   // mail
	DN                string   // 完整 distinguishedName
	Department        string   // department / 部门名
	MemberOf          []string // 所属安全组 DN 列表 (memberOf 属性)
	UserAccountControl int     // userAccountControl 标志位 (0=未设置; 514=禁用; 512=正常)
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

// commonAttrs 返回查询需要拉取的属性列表 (不随查询类型变化)
func (c *adClient) commonAttrs() []string {
	return []string{
		c.cfg.AttrUsername, c.cfg.AttrDisplayName, c.cfg.AttrEmail,
		c.cfg.AttrDN, c.cfg.AttrOrg,
		c.cfg.GroupAttr, // memberOf 或 tokenGroups
		"userAccountControl",
	}
}

// mapEntry 将 LDAP 条目映射为 DirUser
func (c *adClient) mapEntry(e *ldap.Entry) *DirUser {
	uac, _ := strconv.Atoi(e.GetAttributeValue("userAccountControl"))
	u := &DirUser{
		Username:           e.GetAttributeValue(c.cfg.AttrUsername),
		DisplayName:        e.GetAttributeValue(c.cfg.AttrDisplayName),
		Email:              e.GetAttributeValue(c.cfg.AttrEmail),
		DN:                 e.GetAttributeValue(c.cfg.AttrDN),
		Department:         e.GetAttributeValue(c.cfg.AttrOrg),
		MemberOf:           e.GetAttributeValues(c.cfg.GroupAttr),
		UserAccountControl: uac,
	}
	if u.DN == "" {
		u.DN = e.DN
	}
	if u.Username == "" {
		return nil
	}
	return u
}

// buildSearchFilter 构造 LDAP 搜索过滤器。
// groupDNs 非空时: (&(objectClass=user)(|<memberOf 匹配规则>)(groupDN1)(groupDN2)...))
// groupDNs 为空时: (objectClass=user) (兼容旧行为)
// 所有 group DN 均经 ldap.EscapeFilter 转义防注入。
// 递归模式使用 LDAP_MATCHING_RULE_IN_CHAIN (OID 1.2.840.113556.1.4.1941) 展开嵌套组。
func (c *adClient) buildSearchFilter(groupDNs []string) string {
	if len(groupDNs) == 0 {
		return "(objectClass=user)"
	}
	// 转义所有组 DN 防注入
	matchRule := ""
	if c.cfg.SyncRecursive {
		matchRule = ":1.2.840.113556.1.4.1941:"
	}
	var b strings.Builder
	b.WriteString("(&(objectClass=user)(|")
	for _, dn := range groupDNs {
		escaped := ldap.EscapeFilter(dn)
		b.WriteString("(memberOf")
		b.WriteString(matchRule)
		b.WriteString("=")
		b.WriteString(escaped)
		b.WriteString(")")
	}
	b.WriteString("))")
	return b.String()
}

// searchWithPaging 分页搜索: 当 cfg.PageSize > 0 时使用 ControlPaging 逐页拉取,
// 避免 AD MaxPageSize=1000 静默截断。PageSize <= 0 则单次拉取 (兼容旧行为)。
func (c *adClient) searchWithPaging(conn *ldap.Conn, req *ldap.SearchRequest) ([]*ldap.Entry, error) {
	pageSize := c.cfg.PageSize
	if pageSize <= 0 {
		res, err := conn.Search(req)
		if err != nil {
			return nil, fmt.Errorf("ldap search: %w", err)
		}
		return res.Entries, nil
	}
	if pageSize > 1000 {
		pageSize = 1000 // AD 硬上限
	}

	var all []*ldap.Entry
	cookie := []byte(nil)
	for {
		pageReq := req
		ctrl := ldap.NewControlPaging(uint32(pageSize))
		if cookie != nil {
			ctrl.SetCookie(cookie)
		}
		pageReq.Controls = append([]ldap.Control{ctrl}, pageReq.Controls...)

		res, err := conn.Search(pageReq)
		if err != nil {
			return nil, fmt.Errorf("ldap paged search: %w", err)
		}
		all = append(all, res.Entries...)

		// 检查分页 cookie
		var newCookie []byte
		for _, ctrl := range res.Controls {
			if pc, ok := ctrl.(*ldap.ControlPaging); ok {
				newCookie = pc.Cookie
				break
			}
		}
		if len(newCookie) == 0 {
			break
		}
		cookie = newCookie
	}
	slog.Debug("ldap paged search complete", "pages", (len(all)+pageSize-1)/pageSize, "total", len(all))
	return all, nil
}

// SearchUsers 按启用的组 DN 过滤用户, 支持分页。
// groupDNs 为空时回退到全量拉取 (兼容旧行为, 但不推荐)。
// 返回条目含 memberOf 和 userAccountControl。
func (c *adClient) SearchUsers(ctx context.Context, groupDNs []string) ([]DirUser, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	filter := c.buildSearchFilter(groupDNs)

	req := ldap.NewSearchRequest(
		c.cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		c.commonAttrs(),
		nil,
	)
	entries, err := c.searchWithPaging(conn, req)
	if err != nil {
		return nil, err
	}
	users := make([]DirUser, 0, len(entries))
	for _, e := range entries {
		u := c.mapEntry(e)
		if u != nil {
			users = append(users, *u)
		}
	}
	slog.Info("ldap sync: fetched users", "count", len(users), "filter_groups", len(groupDNs))
	return users, nil
}

// SearchOne 用 cfg.UserFilter 精确查询 username 对应的单个用户条目。
// 登录路径使用, 避免拉全量目录; username 经 ldap.EscapeFilter 转义防注入。
// UserFilter 中的 %s 占位符被替换为转义后的 username; 若配置不含 %s 则追加默认过滤。
// 找不到返回 (nil, nil); 配置/目录错误返回 error。
// 返回条目含 memberOf 和 userAccountControl。
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
		c.commonAttrs(),
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
	u := c.mapEntry(e)
	if u == nil {
		return nil, nil
	}
	return u, nil
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
