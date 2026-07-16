#!/usr/bin/env python3
"""
Asset Database System — Demo v0.1
==================================
零依赖概念验证: Python stdlib (http.server + sqlite3 + json)
架构模式对应生产: Go + Gin + PostgreSQL 16 + Redis 7

启动: python3 demo.py
访问: http://localhost:8080/
"""

import http.server
import json
import sqlite3
import time
import uuid
import hashlib
import base64
import urllib.parse
from datetime import datetime, timezone
from contextlib import contextmanager

DB = "demo.db"

# ============================================================================
# 数据库初始化
# ============================================================================

def init_db():
    with get_db() as db:
        db.executescript("""
        PRAGMA journal_mode=WAL;
        PRAGMA foreign_keys=ON;

        CREATE TABLE IF NOT EXISTS organizations (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            parent_id TEXT REFERENCES organizations(id),
            created_at TEXT DEFAULT (datetime('now'))
        );

        CREATE TABLE IF NOT EXISTS users (
            id TEXT PRIMARY KEY,
            username TEXT UNIQUE NOT NULL,
            role TEXT NOT NULL DEFAULT 'viewer'
                CHECK (role IN ('super_admin','admin','manager','viewer','agent')),
            org_id TEXT REFERENCES organizations(id),
            created_at TEXT DEFAULT (datetime('now'))
        );

        CREATE TABLE IF NOT EXISTS asset_types (
            id TEXT PRIMARY KEY,
            name TEXT UNIQUE NOT NULL,
            category TEXT NOT NULL
                CHECK (category IN ('hardware','software','network','cloud_resource','license','other')),
            schema_json TEXT DEFAULT '{}'
        );

        CREATE TABLE IF NOT EXISTS assets (
            id TEXT PRIMARY KEY,
            asset_tag TEXT UNIQUE NOT NULL,
            name TEXT NOT NULL,
            type_id TEXT NOT NULL REFERENCES asset_types(id),
            org_id TEXT NOT NULL REFERENCES organizations(id),
            serial_number TEXT,
            manufacturer TEXT,
            model TEXT,
            lifecycle_state TEXT NOT NULL DEFAULT 'procurement'
                CHECK (lifecycle_state IN ('procurement','deployment','utilization','maintenance','retirement')),
            status TEXT NOT NULL DEFAULT 'available',
            properties TEXT DEFAULT '{}',
            version INTEGER NOT NULL DEFAULT 1,
            deleted_at TEXT,
            created_at TEXT DEFAULT (datetime('now')),
            updated_at TEXT DEFAULT (datetime('now'))
        );

        CREATE INDEX IF NOT EXISTS idx_assets_deleted ON assets(deleted_at);
        CREATE INDEX IF NOT EXISTS idx_assets_org ON assets(org_id, status);

        CREATE TABLE IF NOT EXISTS assignments (
            id TEXT PRIMARY KEY,
            asset_id TEXT NOT NULL REFERENCES assets(id),
            assigned_to TEXT NOT NULL REFERENCES users(id),
            status TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','returned')),
            assigned_at TEXT DEFAULT (datetime('now')),
            returned_at TEXT
        );

        CREATE UNIQUE INDEX IF NOT EXISTS idx_active_assignment
            ON assignments(asset_id) WHERE status='active';

        CREATE TABLE IF NOT EXISTS audit_log (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            asset_id TEXT,
            action TEXT NOT NULL,
            prev_hash TEXT,
            hash TEXT NOT NULL,
            created_at TEXT DEFAULT (datetime('now'))
        );
        CREATE INDEX IF NOT EXISTS idx_audit_asset ON audit_log(asset_id, created_at);

        -- 种子数据
        INSERT OR IGNORE INTO organizations(id,name) VALUES('org-001','Demo Corp');
        INSERT OR IGNORE INTO users(id,username,role,org_id) VALUES('user-001','admin','super_admin','org-001');
        INSERT OR IGNORE INTO asset_types(id,name,category) VALUES
            ('type-001','Laptop','hardware'),
            ('type-002','Server','hardware'),
            ('type-003','Software License','license');
        """)

@contextmanager
def get_db():
    conn = sqlite3.connect(DB)
    conn.row_factory = sqlite3.Row
    try:
        yield conn
        conn.commit()
    except:
        conn.rollback()
        raise
    finally:
        conn.close()

# ============================================================================
# JSON API Handler
# ============================================================================

class APIHandler(http.server.BaseHTTPRequestHandler):
    """REST API — 对应 Go Gin Router"""

    def log_message(self, fmt, *args):
        duration = (time.time() - self.start_time) * 1000
        print(f"[{getattr(self, 'req_id', '-')}] {self.command} {self.path} → {args[0] if args else ''} ({duration:.0f}ms)")

    def _json(self, data, status=200):
        body = json.dumps(data, ensure_ascii=False, default=str).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("X-Request-ID", self.req_id)
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _error(self, status, code, message):
        self._json({"error": {"code": code, "message": message}}, status)

    def _parse_path(self):
        parsed = urllib.parse.urlparse(self.path)
        return parsed.path, dict(urllib.parse.parse_qsl(parsed.query))

    def do_OPTIONS(self):
        self.send_response(204)
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type,If-Match,Authorization")
        self.end_headers()

    def do_GET(self):
        self.start_time = time.time()
        self.req_id = str(uuid.uuid4())[:8]
        path, params = self._parse_path()

        # 健康检查
        if path == "/healthz":
            return self._json({"status": "ok"})

        if path == "/readyz":
            try:
                with get_db() as db:
                    db.execute("SELECT 1")
                return self._json({"status": "ready"})
            except:
                return self._json({"status": "not ready"}, 503)

        # API 路由
        if path == "/api/v1/assets":
            return self._list_assets(params)
        if path.startswith("/api/v1/assets/") and path.endswith("/history"):
            return self._asset_history(path.split("/")[4])
        if path.startswith("/api/v1/assets/"):
            return self._get_asset(path.split("/")[4])

        return self._error(404, "NOT_FOUND", "Route not found")

    def do_POST(self):
        self.start_time = time.time()
        self.req_id = str(uuid.uuid4())[:8]
        path, _ = self._parse_path()
        body = self._read_body()

        if path == "/api/v1/assets":
            return self._create_asset(body)
        if path.startswith("/api/v1/assets/") and path.endswith("/assign"):
            return self._assign_asset(path.split("/")[4])
        if path.startswith("/api/v1/assets/") and path.endswith("/release"):
            return self._release_asset(path.split("/")[4])

        return self._error(404, "NOT_FOUND", "Route not found")

    def do_PUT(self):
        self.start_time = time.time()
        self.req_id = str(uuid.uuid4())[:8]
        path, _ = self._parse_path()
        body = self._read_body()

        if path.startswith("/api/v1/assets/"):
            version = self.headers.get("If-Match", "").strip('"')
            return self._update_asset(path.split("/")[4], body, version)

        return self._error(404, "NOT_FOUND", "Route not found")

    def do_DELETE(self):
        self.start_time = time.time()
        self.req_id = str(uuid.uuid4())[:8]
        path, _ = self._parse_path()

        if path.startswith("/api/v1/assets/"):
            return self._delete_asset(path.split("/")[4])

        return self._error(404, "NOT_FOUND", "Route not found")

    def _read_body(self):
        length = int(self.headers.get("Content-Length", 0))
        if length == 0:
            return {}
        return json.loads(self.rfile.read(length))

    # --- 资产 CRUD ---

    def _list_assets(self, params):
        search = params.get("search", "")
        type_id = params.get("type_id", "")
        cursor = params.get("cursor", "")
        limit = min(int(params.get("limit", 50)), 200)

        conditions = ["deleted_at IS NULL", "org_id = 'org-001'"]
        values = []

        if search:
            conditions.append("(name LIKE ? OR asset_tag LIKE ?)")
            values.extend([f"%{search}%", f"%{search}%"])
        if type_id:
            conditions.append("type_id = ?")
            values.append(type_id)

        if cursor:
            try:
                decoded = json.loads(base64.urlsafe_b64decode(cursor + "=="))
                conditions.append("(updated_at, id) < (?, ?)")
                values.extend([decoded["u"], decoded["i"]])
            except:
                pass

        where = " AND ".join(conditions)
        with get_db() as db:
            rows = db.execute(
                f"SELECT * FROM assets WHERE {where} ORDER BY updated_at DESC, id DESC LIMIT ?",
                values + [limit + 1]
            ).fetchall()

        has_more = len(rows) > limit
        rows = rows[:limit]

        nc = None
        if has_more and rows:
            last = rows[-1]
            nc = base64.urlsafe_b64encode(
                json.dumps({"u": last["updated_at"], "i": last["id"]}).encode()
            ).decode().rstrip("=")

        return self._json({
            "data": [dict(r) for r in rows],
            "pagination": {"next_cursor": nc, "has_more": has_more},
            "request_id": self.req_id,
        })

    def _get_asset(self, aid):
        with get_db() as db:
            row = db.execute(
                "SELECT * FROM assets WHERE id=? AND deleted_at IS NULL", [aid]
            ).fetchone()
            if not row:
                return self._error(404, "NOT_FOUND", f"Asset {aid} not found")
            return self._json(dict(row))

    def _create_asset(self, body):
        aid = str(uuid.uuid4())
        now = datetime.now(timezone.utc).isoformat()
        with get_db() as db:
            db.execute(
                """INSERT INTO assets (id,asset_tag,name,type_id,org_id,serial_number,
                   manufacturer,model,lifecycle_state,status,properties,version,created_at,updated_at)
                   VALUES (?,?,?,?,?,?,?,?,?,?,?,1,?,?)""",
                [aid, body.get("asset_tag"), body.get("name"), body.get("type_id"),
                 "org-001", body.get("serial_number"), body.get("manufacturer"),
                 body.get("model"), body.get("lifecycle_state", "procurement"),
                 body.get("status", "available"), json.dumps(body.get("properties", {})),
                 now, now]
            )
            self._log_audit(db, aid, "asset.created")
            row = db.execute("SELECT * FROM assets WHERE id=?", [aid]).fetchone()
            return self._json(dict(row), 201)

    def _update_asset(self, aid, body, version_str):
        try:
            version = int(version_str)
        except (ValueError, TypeError):
            return self._error(400, "INVALID_VERSION", "If-Match header required")

        with get_db() as db:
            current = db.execute(
                "SELECT version FROM assets WHERE id=? AND deleted_at IS NULL", [aid]
            ).fetchone()
            if not current:
                return self._error(404, "NOT_FOUND", f"Asset {aid} not found")
            if current["version"] != version:
                return self._json({
                    "error": {"code": "VERSION_CONFLICT",
                              "message": f"Expected v{version}, current v{current['version']}"}
                }, 409)

            now = datetime.now(timezone.utc).isoformat()
            fields = ["updated_at=?", "version=version+1"]
            vals = [now]
            for k in ["name","serial_number","manufacturer","model","lifecycle_state","status"]:
                if k in body:
                    fields.append(f"{k}=?")
                    vals.append(body[k])
            if "properties" in body:
                fields.append("properties=?")
                vals.append(json.dumps(body["properties"]))

            db.execute(
                f"UPDATE assets SET {','.join(fields)} WHERE id=? AND version=?",
                vals + [aid, version]
            )
            self._log_audit(db, aid, "asset.updated")
            row = db.execute("SELECT * FROM assets WHERE id=?", [aid]).fetchone()
            return self._json(dict(row))

    def _delete_asset(self, aid):
        now = datetime.now(timezone.utc).isoformat()
        with get_db() as db:
            result = db.execute(
                "UPDATE assets SET deleted_at=?, updated_at=? WHERE id=? AND deleted_at IS NULL",
                [now, now, aid]
            )
            if result.rowcount == 0:
                return self._error(404, "NOT_FOUND", f"Asset {aid} not found")
            self._log_audit(db, aid, "asset.deleted")
            self.send_response(204)
            self.end_headers()

    # --- 领用/归还 ---

    def _assign_asset(self, aid):
        with get_db() as db:
            asset = db.execute("SELECT status FROM assets WHERE id=? AND deleted_at IS NULL", [aid]).fetchone()
            if not asset:
                return self._error(404, "NOT_FOUND", "")
            if asset["status"] != "available":
                return self._error(409, "NOT_AVAILABLE", f"Status: {asset['status']}")

            db.execute("BEGIN IMMEDIATE")  # 悲观锁模拟
            existing = db.execute(
                "SELECT id FROM assignments WHERE asset_id=? AND status='active'", [aid]
            ).fetchone()
            if existing:
                return self._error(409, "ALREADY_ASSIGNED", "")

            now = datetime.now(timezone.utc).isoformat()
            assign_id = str(uuid.uuid4())
            db.execute(
                "INSERT INTO assignments(id,asset_id,assigned_to,assigned_at) VALUES(?,?,?,?)",
                [assign_id, aid, "user-001", now]
            )
            db.execute("UPDATE assets SET status='assigned',updated_at=? WHERE id=?", [now, aid])
            self._log_audit(db, aid, "asset.assigned")
            return self._json({"assignment_id": assign_id, "status": "active"}, 201)

    def _release_asset(self, aid):
        now = datetime.now(timezone.utc).isoformat()
        with get_db() as db:
            r = db.execute(
                "UPDATE assignments SET status='returned',returned_at=? WHERE asset_id=? AND status='active'",
                [now, aid]
            )
            if r.rowcount == 0:
                return self._error(404, "NO_ACTIVE", "No active assignment")
            db.execute("UPDATE assets SET status='available',updated_at=? WHERE id=?", [now, aid])
            self._log_audit(db, aid, "asset.released")
            return self._json({"status": "released"})

    # --- 审计日志 ---

    def _asset_history(self, aid):
        with get_db() as db:
            rows = db.execute(
                "SELECT * FROM audit_log WHERE asset_id=? ORDER BY created_at DESC LIMIT 50", [aid]
            ).fetchall()
            return self._json({"data": [dict(r) for r in rows]})

    def _log_audit(self, db, asset_id, action):
        last = db.execute(
            "SELECT hash FROM audit_log WHERE asset_id=? ORDER BY id DESC LIMIT 1", [asset_id]
        ).fetchone()
        prev = last["hash"] if last else ""
        raw = f"{prev}|{action}|{datetime.now(timezone.utc).isoformat()}"
        h = hashlib.sha256(raw.encode()).hexdigest()
        db.execute(
            "INSERT INTO audit_log(asset_id,action,prev_hash,hash) VALUES(?,?,?,?)",
            [asset_id, action, prev, h]
        )


# ============================================================================
# 启动
# ============================================================================

if __name__ == "__main__":
    init_db()
    addr = ("0.0.0.0", 8080)
    print("=" * 60)
    print("  Asset Database System — Demo v0.1")
    print("  Python stdlib → 生产 Go+Gin+PostgreSQL 16")
    print("=" * 60)
    print(f"  API: http://localhost:8080/api/v1/assets")
    print(f"  Health: http://localhost:8080/healthz")
    print(f"  Ready: http://localhost:8080/readyz")
    print("-" * 60)
    httpd = http.server.HTTPServer(addr, APIHandler)
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down...")
        httpd.shutdown()
