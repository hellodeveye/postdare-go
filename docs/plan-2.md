# Plan 2:Plan 1 收尾修复 + 仓库结构重构

> 前置:Plan 1(SQLite 默认 + 单二进制 + 首启安全初始化)已实现并通过 review,验收项全部实测通过。
> 本计划处理 review 发现的问题,并把仓库结构调整为「单二进制产品」的规范形态。
>
> 原则:**每个 Step 一个独立 commit,功能修复和结构移动绝不混在同一个 commit**。结构调整的每一步都是行为不变的纯移动,验证手段以 `go build / go test / make` 加冒烟为主。

## Review 结论备忘(2026-07-02)

已实测通过:CGO=0 构建、vet、全部测试、前端 tsc、全新环境冒烟(WAL、secrets 0600、随机密码)、403 改密闭环、重启 JWT 持久化、embed UI(immutable 缓存 / SPA 深链 / API 404 JSON / 占位页降级)。

待处理问题:

| # | 级别 | 问题 |
|---|------|------|
| 1 | Bug | 仓库 config.yaml 的 `data_dir: /data/postdare-go` 在 macOS 开发机上无法创建(根目录只读),`go run ./cmd/server` 直接 panic |
| 2 | 运维 | copydb 必须在 SQLite 首次启动服务之前执行(服务先启动会 seed admin,`ensureTargetEmpty` 拒绝),runbook 未写明顺序 |
| 3 | Minor | `RequirePasswordReady` 每个 secured 请求多一次 users 查询 —— **不处理**,单人规模无感;未来在意时把标志放进 JWT claim |
| 4 | Minor | `POSTDARE_GO_DATA_DIR` 只在 db path / log dir 等于内置默认值时联动搬迁 —— **不改行为**,文档写清楚即可 |

---

## Step 0:修复 dev 启动 bug + 提交 Plan 1 工作

**内容**:

1. 修改 `backend/config.yaml`(开发用配置):
   - `data_dir: "./data"`(`.gitignore` 已有 `data/` 规则,不会误提交);
   - `database.path` 删除或留空,由 data_dir 推导;
   - 占位 secret 保持不变(会自动生成到 `./data/secrets.yaml`)。
2. `docs/deployment.md` 补充 copydb 搬迁顺序(问题 2):停服 → `copydb --from <mysql-dsn> --to <path>` → 核对行数 → 用 sqlite 配置启动;如目标库已被服务初始化过,删除 db 文件(含 `-wal`/`-shm`)重跑。
3. README 开发章节补一句:自定义数据目录用 `POSTDARE_GO_DATA_DIR`;并说明问题 4 的联动规则(仅默认路径跟随 data_dir)。
4. 将 Plan 1 全部改动 + 本步修复作为一个(或数个)commit 提交。

**验证**:

- [ ] macOS 开发机上,仓库根 `cd backend && go run ./cmd/server` 直接启动成功,`backend/data/` 下生成 `postdare.db` 与 `secrets.yaml`(0600)。
- [ ] `git status` 干净;`make test` 通过。

---

## Step 1:Go module 提根 + `frontend` → `web`

**目标**:go.mod 位于仓库根,module path 规范化,目录名与「前端是构建资产来源」的定位一致。

**内容**:

1. `backend/go.mod` 移到仓库根,module 改名:
   `postdare-go/backend` → `github.com/hellodeveye/postdare-go`(与 origin 一致,可 `go install`)。
2. `backend/cmd` → `cmd`,`backend/internal` → `internal`(`git mv` 保留历史)。
3. 全量替换 import 前缀(约 40 处):
   `postdare-go/backend/internal/...` → `github.com/hellodeveye/postdare-go/internal/...`。
4. `git mv frontend web`;`backend/config.yaml` → 仓库根 `config.yaml`(或 `configs/config.yaml`,注意 `config.Load` 默认相对路径行为,建议同时把默认查找路径调整为「二进制同目录或当前目录」并在 README 写明)。
5. 同步更新路径引用:
   - `Makefile`:`WEB_DIST := internal/webui/dist`、构建命令去掉 `cd backend`、`cd frontend` → `cd web`;
   - `.gitignore`:`backend/*` 前缀规则、`frontend/dist` → `web/dist` 等;
   - `docs/*.md`、`README.md`、`examples/*.service`、`AGENTS.md` 中的路径;
   - `internal/webui` 的 embed 路径不变(包内相对路径,无需改动)。

**验证**:

- [ ] `go build ./... && go vet ./... && go test ./...`(仓库根执行)全部通过。
- [ ] `make web && make build` 产出 `bin/postdare-go`,启动后 UI 与 API 冒烟正常(登录、项目列表、SSE 日志流)。
- [ ] `grep -rn "postdare-go/backend" --include="*.go" .` 零匹配;`grep -rn "frontend/" Makefile docs README.md` 无残留(plan 文档中的历史记录除外)。
- [ ] `git log --follow internal/handler/handler.go` 能追溯到移动前历史。

---

## Step 2:cmd 合并为单二进制子命令

**目标**:发布物从 3 个二进制(server / mcp-server / copydb)收敛为 1 个。

**内容**:

1. 新建 `cmd/postdare-go/main.go`,用标准库按 `os.Args[1]` 分发(**不引入 cobra**,保持轻量):
   - `serve`(**无参数时的默认**,兼容 systemd 直接执行)→ 现 `cmd/server` 逻辑;
   - `mcp` → 现 `cmd/mcp-server` 逻辑;
   - `copydb --from ... --to ...` → 现 `cmd/copydb` 逻辑(用 `flag.NewFlagSet` 解析子命令参数);
   - `version` → 打印版本后退出;未知子命令打印 usage、退出码 2。
2. 各子命令的 main 逻辑下沉为可调用函数(如 `internal/cli` 或各自小包),`main.go` 只做分发;`-X main.version` 注入点随之确认仍生效。
3. 删除 `cmd/server`、`cmd/mcp-server`、`cmd/copydb`。
4. 更新 `Makefile`(构建目标改为 `./cmd/postdare-go`)、`examples/postdare-go.service`(ExecStart 显式写 `postdare-go serve`)、`docs/mcp.md`(MCP 客户端命令改为 `postdare-go mcp`)、README。

**验证**:

- [ ] `bin/postdare-go`(无参数)与 `bin/postdare-go serve` 均正常启动 HTTP 服务。
- [ ] `bin/postdare-go version` 输出 git describe 版本(经 `make build` 注入)。
- [ ] `POSTDARE_GO_BASE_URL=... POSTDARE_GO_API_TOKEN=... bin/postdare-go mcp` 可作为 stdio MCP server 被客户端连接,`list_projects` 工具可用。
- [ ] `bin/postdare-go copydb` 缺参数时打印 usage 且退出码 2。
- [ ] `bin/postdare-go nonsense` 打印 usage 且退出码 2。
- [ ] `ls cmd/` 只剩 `postdare-go`;`make release` 只产出每平台 1 个二进制。

---

## Step 3:大文件拆分(同包内,零行为变化)

**目标**:消除两个单文件热点,不改包名、不改任何 import。

**内容**:

1. `internal/handler/handler.go`(1085 行)按资源拆为:
   - `router.go`(`RegisterRoutes` + `Handler` 结构体)
   - `auth.go`(login / me / logout / change-password / `RequirePasswordReady`)
   - `projects.go`、`tasks.go`(deploy-tasks 列表/详情/取消/分析)、`logs.go`(部署日志 + 应用日志 + SSE)、`webhooks.go`(入站 webhook + 事件查询)、`dashboard.go`、`settings.go`、`version.go`
2. `internal/service/service.go`(675 行)拆为:
   - `service.go`(`Service` 结构体、`New`、任务注册/取消/关闭)
   - `task.go`(`CreateDeployTask` / `ExecuteTask` / `executeDeploy` / `executeRollback` / 状态收尾)
   - `stages.go`(阶段编排:command / 配置解码 / deferred 逻辑)
   - `healthcheck.go`、`outbound_webhook.go`
3. 拆分只允许移动代码与调整文件内顺序,禁止顺手重命名或改逻辑(diff 应当近似纯移动)。

**验证**:

- [ ] `go build ./... && go test ./...` 通过,测试文件无需改动(同包)。
- [ ] `wc -l internal/handler/*.go internal/service/*.go` 单文件均低于约 400 行。
- [ ] `git diff --stat` 中新旧文件行数总和基本守恒(允许少量 import 块差异)。

---

## Step 4(可选,建议随功能迭代顺势做):包重命名与领域归拢

**目标**:按领域组织,给后续迭代(latest-wins 排队、工件化回滚、SSH runner)一个稳定的演进位置。**本步收益最小、动静最大,不必急于执行;做「部署排队/回滚工件化」时顺势进行最划算。**

**内容**:

1. `internal/handler` + `internal/middleware` + `internal/sse` → `internal/httpapi`(sse 若被 service 依赖,则保留独立包,仅合并前两者)。
2. `internal/service` + `internal/runner` → `internal/deploy`(发布引擎:编排 + 阶段 + 执行,未来的队列与 SSH runner 都落在此包)。
3. `internal/db` → `internal/store`;`internal/webhook` → `internal/gitwebhook`;`internal/notifier` → `internal/notify`。
4. `internal/util` 逐步消化进使用方,消灭杂物包。
5. 每次只动一个包,单独 commit。

**验证**(每个包移动后重复):

- [ ] `go build ./... && go test ./...` 通过。
- [ ] `grep -rn "internal/<旧包名>"` 零匹配。
- [ ] `make build` 后完整冒烟一次:登录 → 触发一次手动部署 → SSE 日志 → 部署历史。

---

## 明确不做的事

- 前端 `web/src` 内部结构**不动**:api / components / hooks / layouts / pages / router / store 的划分对当前规模(约 10 个页面)刚好;仅当 `api/postdareGo.ts` 继续膨胀时按资源拆文件。
- 不引入 cobra / viper / wire 等框架依赖 —— 轻量级定位优先。
- 问题 3(每请求一次 users 查询)不优化。

## 执行顺序与工作量

| Step | 内容 | 预估 | 备注 |
|------|------|------|------|
| 0 | 修 dev 启动 bug + 提交 Plan 1 | 0.5 小时 | 必须最先做 |
| 1 | module 提根 + web 改名 | 1~2 小时 | 机械替换为主,验证要充分 |
| 2 | cmd 合并子命令 | 1~2 小时 | 交付物简化的关键一步 |
| 3 | 大文件拆分 | 1 小时 | 零风险 |
| 4 | 包重命名归拢 | 随功能迭代 | 可无限期推迟 |

全部完成后的形态:仓库根即 Go 项目,`make release` 产出每平台一个 `postdare-go` 二进制,`postdare-go serve / mcp / copydb / version` 覆盖全部入口,internal 无超 400 行的单文件。
