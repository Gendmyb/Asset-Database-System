// Package config — 配置加载 (环境变量)
// 对应架构文档 §2.3
// Phase B: 移除 Redis/Vault/FailOpen 配置 (已砍)
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Auth      AuthConfig
	LDAP      LDAPConfig
	Scheduler SchedulerConfig
	Notify    NotifyConfig
	// DataScope Wave 2 G9: 部门级行级数据权限开关
	// 默认 off → 行为与历史一致 (仅按用户 org_id 过滤);
	// on → 非超级管理员仅可见本部门及子孙部门数据, super_admin 全局可见。
	DataScope DataScopeConfig
}

// DataScopeConfig 行级数据权限配置 (Wave 2 G9)
type DataScopeConfig struct {
	Department bool // DATA_SCOPE_DEPARTMENT=true 启用部门级可见范围
}

// NotifyConfig 通知渠道配置 (Wave 2 G6)
// 所有渠道默认禁用; 未配置凭据/URL 时该渠道不可用, 系统行为与 v0.2.0 一致。
type NotifyConfig struct {
	Enable bool // 总开关; false 时 dispatcher 不投递任何通知

	SMTP SMTPConfig

	// 机器人 webhook URL (系统级, 单条); admin 也可在 notify_rules 里按事件配置
	DingTalkWebhook string
	WeComWebhook    string
	FeishuWebhook   string
}

// SMTPConfig SMTP 邮件发送配置
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string // 从环境变量 SMTP_PASSWORD 读取, 不入日志/审计
	From     string // 发件人地址
}

// SchedulerConfig 调度器配置 (Wave 1 G4)
// Interval <= 0 时调度器不启动 (默认 off, 生产显式开启)。
type SchedulerConfig struct {
	Interval     time.Duration // 扫描间隔; 0 = 不启动
	WarrantyDays int           // 质保临近到期阈值 (默认 30)
	EnableLDAP   bool          // 是否在循环中调用 LDAP 同步
}

// LDAPConfig AD/LDAP 集成配置
// 未配置 (Enable=false) 时系统以纯本地模式运行, 不依赖任何目录服务。
type LDAPConfig struct {
	Enable       bool   // 是否启用 LDAP 登录与同步
	Host         string // AD/LDAP 主机 (e.g. ldap.corp.local)
	Port         int    // 端口 (389 明文 / 636 LDAPS)
	UseTLS       bool   // 启用 StartTLS (port 389 推荐)
	UseSSL       bool   // LDAPS (port 636)
	BindDN       string // 服务账号 DN (用于搜索用户/组, 禁止日志打印其密码)
	BindPassword string // 服务账号密码 (从环境变量 LDAP_BIND_PASSWORD 读取)
	BaseDN       string // 搜索根 DN (e.g. dc=corp,dc=local)
	UserFilter   string // 用户搜索过滤器, %s 占位符替换为用户名
	// 字段映射 (默认走 AD 标准属性名)
	AttrUsername    string // sAMAccountName
	AttrDisplayName string // displayName
	AttrEmail       string // mail
	AttrDN          string // distinguishedName
	AttrOrg         string // department
	// 离职处理: 同步时 AD 不再返回的用户做软删除
	SyncDisabledOnly bool // true 则仅禁用不软删除 (保留可登录历史), 默认 false=软删除

	// === Wave 3 T0 新增: 企业化同步控制 ===
	// PageSize LDAP 分页大小 (ControlPaging), 默认 500; 0 或负数=不分页 (最大 1000)
	PageSize int
	// SyncRecursive 是否递归展开组成员 (LDAP_MATCHING_RULE_IN_CHAIN OID 1.2.840.113556.1.4.1941)
	// true=递归 (含嵌套组), false=仅直接成员 (默认, 性能更优)
	SyncRecursive bool
	// LinkExisting 同步时是否自动链接同名本地账号 (保留其 id/角色/历史)
	// 默认 false=仅跳过冲突, true=将 source='local' 的同名用户更新为 source='ldap'
	LinkExisting bool
	// GroupAttr 组成员属性名 (默认 memberOf, 也可配为 tokenGroups 等)
	GroupAttr string
}

type ServerConfig struct {
	Host            string        `default:"0.0.0.0"`
	Port            string        `default:"8080"`
	ReadTimeout     time.Duration `default:"30s"`
	WriteTimeout    time.Duration `default:"30s"`
	ShutdownTimeout time.Duration `default:"10s"`
	// ExternalURL 受信基础 URL (e.g. https://assets.corp.local), 用于二维码等
	// 需要回拼前端 URL 的场景。为空时禁止 url 模式 QR 生成 (防 Host 头注入钓鱼)。
	ExternalURL string
}

type DatabaseConfig struct {
	Host            string        `default:"localhost"`
	Port            string        `default:"5432"`
	Name            string        `default:"assetdb"`
	User            string        `default:"app_writer"`
	Password        string        // 从环境变量 DATABASE_PASSWORD 读取
	Schema          string        `default:"assets"`
	MaxConns        int           `default:"25"`
	MinConns        int           `default:"5"`
	MaxConnLifetime time.Duration `default:"1h"`
	MaxConnIdleTime time.Duration `default:"10m"`
	AutoMigrate     bool          `default:"false"` // 开发环境 true, 生产 false
}

type AuthConfig struct {
	AccessTokenTTL  time.Duration `default:"15m"`
	RefreshTokenTTL time.Duration `default:"7d"`
	Issuer          string        `default:"asset-db-api"`
	Audience        string        `default:"asset-db"`
	Ed25519Seed     string        // 从环境变量 JWT_ED25519_SEED 读取 (64 hex chars)
}

// 已移除: RedisConfig, VaultConfig, FailOpen* — Phase B 砍掉, 后续按需恢复

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host:            getEnv("SERVER_HOST", "0.0.0.0"),
			Port:            getEnv("SERVER_PORT", "8080"),
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
			ExternalURL:     strings.TrimRight(getEnv("EXTERNAL_URL", ""), "/"),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "5432"),
			Name:            getEnv("DB_NAME", "assetdb"),
			User:            getEnv("DB_USER", "app_user"),
			Password:        os.Getenv("DATABASE_PASSWORD"),
			Schema:          "assets",
			MaxConns:        25,
			MinConns:        5,
			MaxConnLifetime: 1 * time.Hour,
			MaxConnIdleTime: 10 * time.Minute,
		},
		Auth: AuthConfig{
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "asset-db-api",
			Audience:        "asset-db",
			Ed25519Seed:     os.Getenv("JWT_ED25519_SEED"),
		},
		LDAP: loadLDAPConfig(),
		Scheduler: SchedulerConfig{
			Interval:     loadSchedulerInterval(),
			WarrantyDays: getEnvInt("SCHEDULER_WARRANTY_DAYS", 30),
			EnableLDAP:   getEnvBool("SCHEDULER_LDAP_SYNC", false),
		},
		Notify: loadNotifyConfig(),
		DataScope: DataScopeConfig{
			Department: getEnvBool("DATA_SCOPE_DEPARTMENT", false),
		},
	}
	return cfg, nil
}

// loadNotifyConfig 从环境变量读取通知配置
// SMTP 主机未配置则 SMTP 渠道禁用; 任一机器人 URL 配置即启用该渠道。
// 总开关 NOTIFY_ENABLE 默认 false (向后兼容: 关闭时系统行为与 v0.2.0 一致)。
func loadNotifyConfig() NotifyConfig {
	cfg := NotifyConfig{
		Enable:          getEnvBool("NOTIFY_ENABLE", false),
		DingTalkWebhook: getEnv("NOTIFY_DINGTALK_WEBHOOK", ""),
		WeComWebhook:    getEnv("NOTIFY_WECOM_WEBHOOK", ""),
		FeishuWebhook:   getEnv("NOTIFY_FEISHU_WEBHOOK", ""),
		SMTP: SMTPConfig{
			Host:     getEnv("SMTP_HOST", ""),
			Port:     getEnvInt("SMTP_PORT", 587),
			User:     getEnv("SMTP_USER", ""),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     getEnv("SMTP_FROM", ""),
		},
	}
	return cfg
}

// loadSchedulerInterval 解析 env SCHEDULER_INTERVAL
// 默认 "off" (不启动); 支持 "30m" / "1h" / "24h" 等 Go duration, 或纯数字秒。
func loadSchedulerInterval() time.Duration {
	v := getEnv("SCHEDULER_INTERVAL", "off")
	if v == "" || v == "off" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err == nil {
		return d
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// loadLDAPConfig 从环境变量读取 LDAP 配置
// 任一关键字段缺失则 Enable=false (系统以纯本地模式运行)
func loadLDAPConfig() LDAPConfig {
	cfg := LDAPConfig{
		Host:             getEnv("LDAP_HOST", ""),
		Port:             getEnvInt("LDAP_PORT", 389),
		UseTLS:           getEnvBool("LDAP_USE_TLS", false),
		UseSSL:           getEnvBool("LDAP_USE_SSL", false),
		BindDN:           getEnv("LDAP_BIND_DN", ""),
		BindPassword:     os.Getenv("LDAP_BIND_PASSWORD"),
		BaseDN:           getEnv("LDAP_BASE_DN", ""),
		UserFilter:       getEnv("LDAP_USER_FILTER", "(&(objectClass=user)(sAMAccountName=%s))"),
		AttrUsername:     getEnv("LDAP_ATTR_USERNAME", "sAMAccountName"),
		AttrDisplayName:  getEnv("LDAP_ATTR_DISPLAY_NAME", "displayName"),
		AttrEmail:        getEnv("LDAP_ATTR_EMAIL", "mail"),
		AttrDN:           getEnv("LDAP_ATTR_DN", "distinguishedName"),
		AttrOrg:          getEnv("LDAP_ATTR_ORG", "department"),
		SyncDisabledOnly: getEnvBool("LDAP_SYNC_DISABLE_ONLY", false),
		// Wave 3 T0: 企业化同步控制
		PageSize:      getEnvInt("LDAP_PAGE_SIZE", 500),
		SyncRecursive: getEnvBool("LDAP_SYNC_RECURSIVE", false),
		LinkExisting:  getEnvBool("LDAP_LINK_EXISTING", false),
		GroupAttr:     getEnv("LDAP_GROUP_ATTR", "memberOf"),
	}
	// 仅在关键字段齐全时启用 LDAP
	if cfg.Host != "" && cfg.BaseDN != "" && cfg.BindDN != "" {
		cfg.Enable = true
	}
	return cfg
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func (d *DatabaseConfig) DSN() string {
	sslMode := os.Getenv("DB_SSLMODE")
	if sslMode == "" {
		sslMode = "disable"
	}
	return "postgres://" + d.User + ":" + d.Password +
		"@" + d.Host + ":" + d.Port + "/" + d.Name +
		"?sslmode=" + sslMode + "&search_path=" + d.Schema
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
