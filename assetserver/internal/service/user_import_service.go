// Package service — 用户批量 CSV 导入
// Wave 1 G2: 复用 import_service.go 的 dry-run + 逐行报错 + 事务导入模式
//
// CSV 列: username,display_name,email,role,org_path,password(可选)
//   - username: 必填, 全局唯一 (含软删除行, 因 users.username UNIQUE)
//   - role: 必填, 须满足 users.role CHECK (super_admin/admin/manager/viewer)
//   - org_path: 可选, ltree path (e.g. root.技术部) 或组织 id; 留空取默认 org
//   - password: 可选, 留空则生成随机密码并随结果返回 (不写审计)
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// UserImportService 用户批量导入服务
type UserImportService struct{}

// NewUserImportService 构造服务
func NewUserImportService() *UserImportService {
	return &UserImportService{}
}

// userImportColumns 模板列
var userImportColumns = []string{
	"username", "display_name", "email", "role", "org_path", "password",
}

// UserImportRowError 单行错误
type UserImportRowError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// UserImportPreview 预览结果
type UserImportPreview struct {
	Total  int                  `json:"total"`
	Valid  int                  `json:"valid"`
	Errors []UserImportRowError `json:"errors"`
}

// UserImportRowResult 单行执行结果 (含生成的密码; 仅返回给调用方, 不入审计)
type UserImportRowResult struct {
	Row      int    `json:"row"`
	Username string `json:"username"`
	Role     string `json:"role"`
	OrgID    string `json:"org_id"`
	Status   string `json:"status"` // "created" | "error"
	Error    string `json:"error,omitempty"`
	// GeneratedPassword: 当 CSV 未提供密码时生成的随机密码。
	// 仅随结果返回给调用 admin; 不写入审计/日志。
	GeneratedPassword string `json:"generated_password,omitempty"`
}

// UserImportResult 执行结果
type UserImportResult struct {
	Total   int                   `json:"total"`
	Created int                   `json:"created"`
	Errors  int                   `json:"errors"`
	Rows    []UserImportRowResult `json:"rows"`
}

// validRoles 合法角色集合 (须匹配 users.role CHECK 约束)
var validRoles = map[string]bool{
	"super_admin": true,
	"admin":       true,
	"manager":     true,
	"viewer":      true,
}

// defaultOrgID 兜底组织
const defaultOrgID = "00000000-0000-4000-a000-000000000001"

// GetUserImportTemplate 返回 UTF-8 BOM CSV 模板
func (s *UserImportService) GetUserImportTemplate() []byte {
	var buf strings.Builder
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM
	w := csv.NewWriter(&buf)
	_ = w.Write(userImportColumns)
	// 示例行
	_ = w.Write([]string{"jdoe", "John Doe", "jdoe@corp.local", "viewer", "root.技术部", ""})
	w.Flush()
	return []byte(buf.String())
}

// PreviewImport 预览 CSV 导入 (只校验, 不写库)
func (s *UserImportService) PreviewImport(ctx context.Context, q repository.DBTX, reader io.Reader) (*UserImportPreview, error) {
	records, err := readUserCSV(reader)
	if err != nil {
		return nil, err
	}
	result := &UserImportPreview{Errors: make([]UserImportRowError, 0)}

	for _, rec := range records {
		result.Total++
		if fieldErr := validateUserRow(ctx, q, rec.fields); fieldErr != "" {
			result.Errors = append(result.Errors, UserImportRowError{
				Row: rec.rowNum, Field: fieldErr, Message: fieldMessage(fieldErr, rec.fields),
			})
			continue
		}
		result.Valid++
	}
	return result, nil
}

// ExecuteImport 执行 CSV 导入 (事务内逐行创建 + 审计)
// actorID: 触发导入的 admin id; actorOrgID: 用于审计归属。
// 返回结果含生成密码列表, 调用方应一次性返回给前端, 不写审计/日志。
func (s *UserImportService) ExecuteImport(
	ctx context.Context, pool *pgxpool.Pool, actorID, actorOrgID string, reader io.Reader,
) (*UserImportResult, error) {
	records, err := readUserCSV(reader)
	if err != nil {
		return nil, err
	}
	result := &UserImportResult{Total: len(records), Rows: make([]UserImportRowResult, 0, len(records))}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, rec := range records {
		rr := UserImportRowResult{Row: rec.rowNum, Username: rec.fields["username"], Role: rec.fields["role"]}

		// 校验
		if fieldErr := validateUserRow(ctx, tx, rec.fields); fieldErr != "" {
			rr.Status = "error"
			rr.Error = fieldMessage(fieldErr, rec.fields)
			result.Errors++
			result.Rows = append(result.Rows, rr)
			continue
		}

		// 解析 org
		orgID := defaultOrgID
		if op := rec.fields["org_path"]; op != "" {
			if id, err := resolveOrg(ctx, tx, op); err == nil {
				orgID = id
			}
		}
		rr.OrgID = orgID

		// 密码: CSV 提供则用, 否则随机生成 (16 字节 base64)
		password := rec.fields["password"]
		genPwd := ""
		if password == "" {
			genPwd = generateRandomPassword(16)
			password = genPwd
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			rr.Status = "error"
			rr.Error = "密码加密失败"
			result.Errors++
			result.Rows = append(result.Rows, rr)
			continue
		}

		// 插入用户 (source='local', must_change_password=true 强制首登改密)
		var userID string
		err = tx.QueryRow(ctx,
			`INSERT INTO assets.users
			   (org_id, username, password_hash, role, email, status, source,
			    display_name, must_change_password, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, 'active', 'local', $6, true, now(), now())
			 RETURNING id::text`,
			orgID, rec.fields["username"], string(hash), rec.fields["role"],
			nullable(rec.fields["email"]), nullable(rec.fields["display_name"]),
		).Scan(&userID)
		if err != nil {
			rr.Status = "error"
			if isDuplicate(err) {
				rr.Error = "用户名已存在"
			} else {
				rr.Error = fmt.Sprintf("创建失败: %v", err)
			}
			result.Errors++
			result.Rows = append(result.Rows, rr)
			continue
		}

		// 审计 (不含密码) — 用独立事务写, 失败仅记日志, 不影响导入主事务
		newVals, _ := json.Marshal(map[string]interface{}{
			"id":       userID,
			"username": rec.fields["username"],
			"role":     rec.fields["role"],
			"email":    rec.fields["email"],
			"org_id":   orgID,
			"source":   "local",
		})
		newVals = truncateAuditMetadata(newVals)
		if err := writeUserImportAudit(ctx, pool, audit.Entry{
			TableName: "users",
			RecordID:  userID,
			Action:    audit.ActionCreated,
			OrgID:     actorOrgID,
			ActorID:   actorID,
			NewValues: newVals,
		}); err != nil {
			slog.Warn("user import: write audit failed",
				"username", rec.fields["username"], "error", err)
		}

		rr.Status = "created"
		if genPwd != "" {
			rr.GeneratedPassword = genPwd
		}
		result.Created++
		result.Rows = append(result.Rows, rr)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit import: %w", err)
	}
	return result, nil
}

// --- 内部工具 ---

type userCSVRow struct {
	rowNum int
	fields map[string]string
}

// readUserCSV 读取并解析 CSV; 跳过 BOM 与 # 注释行; 返回每行字段 map。
func readUserCSV(reader io.Reader) ([]userCSVRow, error) {
	r := csv.NewReader(reader)
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	// 允许变长字段 (注释行/尾随空列), 缺列按空串处理
	r.FieldsPerRecord = -1

	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}
	// 去除 BOM (首列首字节)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\xEF\xBB\xBF")
	}
	colIndex := make(map[string]int)
	for i, h := range headers {
		colIndex[strings.TrimSpace(h)] = i
	}
	if _, ok := colIndex["username"]; !ok {
		return nil, fmt.Errorf("missing required column: username")
	}
	if _, ok := colIndex["role"]; !ok {
		return nil, fmt.Errorf("missing required column: role")
	}

	var out []userCSVRow
	rowNum := 1
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv row %d: %w", rowNum+1, err)
		}
		rowNum++
		// 跳过注释行
		if len(record) > 0 && strings.HasPrefix(strings.TrimSpace(record[0]), "#") {
			continue
		}
		m := make(map[string]string, len(colIndex))
		for k, idx := range colIndex {
			if idx >= 0 && idx < len(record) {
				m[k] = strings.TrimSpace(record[idx])
			}
		}
		out = append(out, userCSVRow{rowNum: rowNum, fields: m})
	}
	return out, nil
}

// validateUserRow 校验单行; 返回空串表示通过, 否则返回字段名。
func validateUserRow(ctx context.Context, q repository.DBTX, f map[string]string) string {
	if f["username"] == "" {
		return "username"
	}
	if !validRoles[f["role"]] {
		return "role"
	}
	// email 格式 (若填写)
	if e := f["email"]; e != "" && !strings.Contains(e, "@") {
		return "email"
	}
	// username 全局唯一 (含软删除行)
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM assets.users WHERE username = $1)`,
		f["username"],
	).Scan(&exists); err == nil && exists {
		return "username"
	}
	// org_path 解析 (若填写)
	if op := f["org_path"]; op != "" {
		if _, err := resolveOrg(ctx, q, op); err != nil {
			return "org_path"
		}
	}
	return ""
}

// fieldMessage 根据字段返回中文错误
func fieldMessage(field string, f map[string]string) string {
	switch field {
	case "username":
		if f["username"] == "" {
			return "用户名不能为空"
		}
		return "用户名已存在: " + f["username"]
	case "role":
		return "角色无效: " + f["role"]
	case "email":
		return "邮箱格式错误: " + f["email"]
	case "org_path":
		return "组织不存在: " + f["org_path"]
	}
	return field
}

// resolveOrg 按 ltree path 或 UUID 解析组织 id
func resolveOrg(ctx context.Context, q repository.DBTX, ident string) (string, error) {
	// 尝试 UUID
	var id string
	err := q.QueryRow(ctx,
		`SELECT id::text FROM assets.organizations WHERE id::text = $1 LIMIT 1`, ident,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	// 尝试 path
	err = q.QueryRow(ctx,
		`SELECT id::text FROM assets.organizations WHERE path::text = $1 LIMIT 1`, ident,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	// 尝试 name + 一级组织 (人性化兜底)
	err = q.QueryRow(ctx,
		`SELECT id::text FROM assets.organizations WHERE name = $1 AND depth = 1 LIMIT 1`, ident,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	return "", fmt.Errorf("org not found: %s", ident)
}

// nullable 空串返回 NULL 友好参数
func nullable(s string) string { return s }

// isDuplicate 检查错误是否为唯一约束冲突
func isDuplicate(err error) bool {
	return strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique") ||
		strings.Contains(err.Error(), "23505")
}

// generateRandomPassword 生成 n 字节 base64 编码的随机密码
func generateRandomPassword(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// 不应发生; 退化到时间戳避免返回空
		return fmt.Sprintf("pwd-%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// truncateAuditMetadata 确保 audit metadata::text 不超过 4096 字节 (audit_log CHECK 约束)。
// metadata 列存的是完整 Entry JSON (含 NewValues), 故对 summary 预留 Entry 包装字段空间。
// 超长时替换为占位 JSON, 防止 INSERT 失败导致事务中毒。
func truncateAuditMetadata(raw []byte) []byte {
	// 3500 字节留给 summary, 剩余 ~500 字节给 Entry 包装字段
	const maxSummaryLen = 3500
	if len(raw) <= maxSummaryLen {
		return raw
	}
	return []byte(fmt.Sprintf(`{"truncated":true,"orig_len":%d}`, len(raw)))
}

// writeUserImportAudit 在独立事务中写入审计条目, 与导入主事务解耦。
// 失败仅返回 error 由调用方记日志, 不回滚业务事务 (避免审计失败中毒主事务)。
func writeUserImportAudit(ctx context.Context, pool *pgxpool.Pool, e audit.Entry) error {
	if pool == nil {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin audit tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := audit.Record(ctx, tx, e); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit: %w", err)
	}
	return nil
}
