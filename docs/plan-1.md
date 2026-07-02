# Plan 1:轻量化交付改造(SQLite 默认 + 单二进制 + 首启安全初始化)

> 目标:把 Postdare Go 的安装体验做成「下载一个二进制 → scp → systemd 启动」,零外部依赖,默认配置即安全。
>
> 按三个独立 PR 推进:PR 1 是 PR 2 的前置(CGO=0 交叉编译),PR 3 轻度依赖 PR 1 的数据目录概念。

## 现状基础(已验证)

- `backend/internal/db/db.go` 启动时已执行 `AutoMigrate`,schema 不依赖 SQL 迁移脚本。
- handler / service 测试已全程跑在 SQLite(`sqlite.Open(":memory:")`),model 层 SQLite 兼容性已被测试覆盖。
- `CommandRunner` 等核心接口不受本次改造影响。

## 开工前检查点(已确认,2026-07-02)

- [x] 现有 MySQL 生产库(100.100.150.28:3306 / postdare_go)已应用完 `backend/migrations` 下全部 6 个迁移(截至 `20260702_drop_project_fields.sql`),`schema_migrations` 表已核对。可以安全移除迁移机制。
- [x] 现有 MySQL 数据**需要**搬到 SQLite → PR 1 的步骤 7 `cmd/copydb` 为必做项。搬迁核对基线(2026-07-02 快照):

  | 表 | 行数 |
  |----|------|
  | users | 1 |
  | projects | 3 |
  | deploy_tasks | 62 |
  | deploy_task_stages | 360 |
  | webhook_events | 39 |
  | settings | 0 |

  注:正式搬迁在停服窗口内执行,以当时的实际行数为准重新核对。

---

## PR 1:SQLite 做默认数据库

**目标**:默认零外部依赖启动;MySQL 降级为可选配置;`CGO_ENABLED=0` 可构建。

### 实现步骤

1. **换纯 Go SQLite 驱动**
   - `go.mod` 引入 `github.com/glebarez/sqlite`。
   - 移除 `gorm.io/driver/sqlite`(底层是 CGO 的 `mattn/go-sqlite3`),同步替换 `handler_test.go`、`service_test.go` 中的 import。

2. **扩展 `internal/config/config.go` 的 `DatabaseConfig`**

   ```yaml
   database:
     driver: sqlite                        # 默认值,applyDefaults 中兜底
     path: /data/postdare-go/postdare.db   # driver=sqlite 时生效,默认此路径
     dsn: ""                                # driver=mysql 时必填,沿用现有语义
   ```

   - `applyDefaults()`:`driver` 为空补 `sqlite`,`path` 为空补默认路径。
   - 校验:`driver=mysql` 且 `dsn` 为空时返回明确错误。

3. **改造 `internal/db/db.go` 的 `Open`**
   - 按 `cfg.Driver` 分支:`sqlite` / `mysql`,其他值报错。
   - sqlite 分支:先 `os.MkdirAll(filepath.Dir(path), 0o755)`;连接串追加
     `?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)`。
   - `AutoMigrate` 与 `seedDefaultAdmin` 逻辑不变(两种 driver 共用)。

4. **MySQL 方言审计**
   - `grep -rn 'Raw\|Exec\|Order("' backend --include='*.go'` 逐处检查裸 SQL。
   - 已知一处:`handler.go` 的 `` Order("`key` asc") ``(约 551 行)。SQLite 兼容反引号,可运行,但改为 `Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}})` 或列名重命名,消除方言残留。

5. **移除 SQL 迁移机制**(依赖开工前检查点 1)
   - 删除 `backend/cmd/migrate`、`backend/internal/migration`、`backend/migrations/`。
   - README 中「MySQL 初始化」「已有数据库升级」章节改写为:默认无需任何数据库准备;可选 MySQL 时只需建空库,schema 由启动时 AutoMigrate 创建。

6. **测试适配**
   - 全部测试使用 `glebarez/sqlite`;新增一个 `db.Open` 的单测:临时目录下以 sqlite driver 打开 → 断言 db 文件生成、admin 已 seed、二次打开幂等。

7. **一次性数据搬迁工具(必做)**
   - 新增 `backend/cmd/copydb`:读 MySQL DSN,用共享 model 全量读取 6 张表(User、Project、DeployTask、DeployTaskStage、WebhookEvent、Setting),按主键顺序写入目标 SQLite 文件(保留原主键 ID,任务与阶段的关联不变)。
   - 用法:`go run ./cmd/copydb --from "<mysql-dsn>" --to /data/postdare-go/postdare.db`。
   - 搬迁流程(停服窗口内):停止 postdare-go → 跑 copydb → 逐表核对行数与基线一致 → 用 sqlite 配置启动 → UI 抽查项目 / 部署历史 / webhook 事件 → 确认后 MySQL 库保留一段时间作回退。
   - 迁移完成后此命令可在后续版本删除。

### 验收标准

- [ ] `cd backend && CGO_ENABLED=0 go build ./...` 通过(全仓无 CGO 依赖)。
- [ ] `cd backend && go vet ./... && go test ./...` 通过。
- [ ] 删除(或注释)config.yaml 中 database 配置后 `go run ./cmd/server`:服务正常启动,`postdare.db` 文件按默认路径生成,`admin` 用户已 seed,登录接口可用。
- [ ] `driver: mysql` + 原 DSN 启动,行为与现版本一致(回归验证)。
- [ ] 连续两次启动无报错(AutoMigrate 幂等)。
- [ ] copydb 跑完后,SQLite 库中 6 张表行数与 MySQL 逐表一致(参照开工前检查点的基线表),用 SQLite 启动后项目列表 / 部署历史 / webhook 事件在 UI 上完整可见,原 admin 密码可正常登录。

---

## PR 2:go:embed 单二进制交付

**目标**:一个二进制同时服务 API 和前端静态资源,交付方式为「scp + systemd」。

### 实现步骤

1. **解决跨模块 embed**(`go:embed` 不能引用 backend 模块外的 `frontend/dist`)
   - 新建 `backend/internal/webui/webui.go`:`//go:embed all:dist` + `embed.FS`。
   - 构建脚本负责把 `frontend/dist` 拷贝为 `backend/internal/webui/dist`。
   - `backend/internal/webui/dist/` 加入 `.gitignore`;包内提交一个 `dist/.gitkeep` 或占位 `index.html`,保证未构建前端时 `go build` 不失败,运行时返回「Web UI not embedded, run make release」提示页。

2. **路由接入**(`backend/cmd/server/main.go` 或 handler 包)
   - 注册 `router.NoRoute`:
     - 路径以 `/api` 开头 → 返回 404 JSON(保持 API 错误格式统一);
     - 命中静态文件 → 直接返回,`assets/` 下带 hash 的文件加 `Cache-Control: public, max-age=31536000, immutable`;
     - 未命中 → 回退返回 `index.html`(SPA 深链刷新支持),`index.html` 用 `no-cache`。
   - CORS 中间件保持现状(单二进制下同源,该中间件仅服务开发场景)。

3. **版本注入**
   - `main.go` 增加 `var version = "dev"`,构建时 `-ldflags "-X main.version=..."` 注入。
   - 新增 `GET /api/v1/version`(无需鉴权)返回 `{"version": "...", "go": "..."}`。

4. **构建脚本**:根目录新增 `Makefile`

   ```makefile
   make web      # cd frontend && npm ci && npm run build && 拷贝 dist 到 backend/internal/webui/
   make build    # CGO_ENABLED=0 go build -ldflags 版本注入,产出 bin/postdare-go
   make release  # web + build,交叉编译 linux/amd64 与 linux/arm64
   make test     # cd backend && go vet ./... && go test ./...
   ```

5. **发布流水线(可选,可放本 PR 或独立小 PR)**
   - goreleaser 配置 + GitHub Actions:打 tag 自动构建两个平台产物并挂 Release。

6. **交付文档更新**
   - README 安装章节改写:下载二进制 → scp → `systemctl enable --now`。
   - 核对 `examples/postdare-go.service` 的 ExecStart / WorkingDirectory 与单二进制形态一致。
   - `docs/deployment.md` 同步。

### 验收标准

- [ ] `make release` 一条命令产出 linux/amd64 与 linux/arm64 二进制。
- [ ] 单二进制启动后,浏览器访问 `http://<host>:8088` 直接出登录页(无需 Vite / npm)。
- [ ] 登录、项目 CRUD、SSE 部署日志流、应用日志流全部正常(SSE 走同端口)。
- [ ] 深链刷新不 404:直接刷新 `/deploy-tasks/123` 等前端路由返回页面。
- [ ] `/api/v1/no-such-endpoint` 返回 404 JSON 而非 index.html。
- [ ] `curl /api/v1/version` 返回构建版本号。
- [ ] 未执行 `make web` 时 `go build ./...`、`go test ./...` 仍通过(占位降级生效)。

---

## PR 3:首次启动安全初始化

**目标**:消灭 `please-change-this-secret` 占位 secret 和固定默认密码;默认配置即安全。

### 实现步骤

1. **配置文件变为可选**
   - `config.Load`:文件不存在时不再报错,使用全默认值(显式通过 `POSTDARE_GO_CONFIG` 指定的路径不存在仍报错)。

2. **secrets 与配置分离**
   - 新增数据目录概念(默认 `/data/postdare-go`,与现有日志目录一致)。
   - 启动时若 `jwt.secret` / `mcp.api_token` 为空或等于占位值(`please-change-this-*`),用 `crypto/rand` 生成 32 字节(hex/base64),持久化到 `<data_dir>/secrets.yaml`,文件权限 `0600`。
   - 加载优先级:环境变量 > config.yaml(非占位值)> secrets.yaml > 首次生成。
   - 不回写用户的 config.yaml,机器生成内容只进 secrets.yaml。

3. **环境变量覆盖**(手写 override 函数,不引 viper)
   - `POSTDARE_GO_PORT`、`POSTDARE_GO_DB_DRIVER`、`POSTDARE_GO_DB_PATH`、`POSTDARE_GO_DB_DSN`、`POSTDARE_GO_JWT_SECRET`、`POSTDARE_GO_MCP_API_TOKEN`、`POSTDARE_GO_ADMIN_PASSWORD`。
   - 在 `config.Load` 末尾统一应用,单元测试覆盖优先级。

4. **默认管理员改造**(`internal/db/db.go` 的 `seedDefaultAdmin`)
   - `User` model 新增 `MustChangePassword bool`(AutoMigrate 自动加列,SQLite / MySQL 均覆盖)。
   - 首次 seed:优先取 `POSTDARE_GO_ADMIN_PASSWORD`(此时 `must_change_password=false`);未设置则 `crypto/rand` 生成随机密码,**打印到启动日志一次**,并置 `must_change_password=true`。
   - 移除硬编码 `admin123456`。

5. **补齐改密码闭环**(当前只有 login / me / logout,无改密码接口)
   - 新增 `PUT /api/v1/auth/password`:请求 `{old_password, new_password}`;校验旧密码、最小长度(≥8),bcrypt 存新密码,清 `must_change_password`。
   - 后端强制:`must_change_password=true` 时,除 `GET /auth/me`、`POST /auth/logout`、`PUT /auth/password` 外的 secured 接口返回 `403 PASSWORD_CHANGE_REQUIRED`(在 Auth 中间件或紧随其后的小中间件实现)。
   - `GET /auth/me` 与登录响应带上 `must_change_password` 字段。
   - 前端:登录后检测标志强制进入改密码页;设置页增加常规「修改密码」入口。

6. **启动摘要日志**
   - 启动时打印一段结构化摘要:生效 db driver 与路径、部署日志目录、监听端口、MCP enabled / mutation 开关。secret 类只打印是否已配置,不打印值。

7. **文档更新**
   - README「默认账号」「安全注意事项」章节按新行为重写(从「请记得改」变为「默认即安全」);`docs/rest-api.md` 增补改密码接口。

### 验收标准

- [ ] 全新环境、无 config.yaml 直接启动:服务正常运行,日志输出随机 admin 密码,`<data_dir>/secrets.yaml` 生成且权限为 `0600`。
- [ ] 用日志中的密码登录 → 其他 API 返回 `403 PASSWORD_CHANGE_REQUIRED` → 前端强制改密码 → 改完全部功能恢复。
- [ ] 重启服务后原 JWT 仍有效(secret 已持久化,未重新生成)。
- [ ] 设置 `POSTDARE_GO_JWT_SECRET` 启动时,secrets.yaml 中的值被覆盖且不再改写。
- [ ] `POSTDARE_GO_ADMIN_PASSWORD` 预设密码 seed 后,登录不触发强制改密码。
- [ ] 改密码接口:旧密码错误返回 401 语义错误;新密码过短返回 400;成功后旧密码不可登录。
- [ ] config.yaml 中残留占位 secret(`please-change-this-*`)不会被实际使用(等同于空,走生成逻辑)。
- [ ] `go test ./...` 通过,新增逻辑(config 优先级、seed、改密码、强制拦截)均有单测。

---

## 工作量与顺序

| PR | 内容 | 预估 |
|----|------|------|
| PR 1 | SQLite 默认 + 移除迁移机制 | 0.5 ~ 1 天 |
| PR 2 | go:embed 单二进制 + Makefile | 0.5 天 |
| PR 3 | 首启安全初始化 + 改密码闭环 | 1 天(含前端) |

完成后的最终形态:`make release` 产出单二进制 → scp 到服务器 → `systemctl enable --now postdare-go` → 日志取初始密码登录改密 → 开始配置项目。
