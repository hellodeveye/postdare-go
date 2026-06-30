# AGENTS.md

Compact guidance for OpenCode sessions working in this repo.

## Layout

- `backend/` — Go module `postdare-go/backend` (go 1.22). Two binaries: `cmd/server` (HTTP API on :8088) and `cmd/mcp-server` (stdio MCP that calls the REST API, no DB access). App code lives under `internal/`.
- `frontend/` — Vite + React + TS SPA on :5173. Not a Go workspace; there is no root `go.mod`.
- `docs/` (REST API, webhooks, MCP, deployment), `examples/` (systemd units + deploy/rollback scripts), `PRODUCT.md` / `DESIGN.md` (product + visual intent: dark-first, restrained UI).

## Backend

Run commands from `backend/` — they and `config.yaml` resolve against the CWD.

```bash
go run ./cmd/server          # needs MySQL (database.dsn in config.yaml)
go run ./cmd/mcp-server      # needs POSTDARE_GO_BASE_URL + POSTDARE_GO_API_TOKEN env
go test ./...                # uses sqlite; NO MySQL required
go vet ./...                 # only static check configured
```

- Config path override: `POSTDARE_GO_CONFIG=/path/to/config.yaml`.
- `go test ./...` is safe to run anywhere — tests use in-memory/temp sqlite, never MySQL. Don't skip backend tests assuming a DB is required.
- No Makefile, no CI workflows, no linter config, no pre-commit hooks. The full verification loop is `go vet ./... && go test ./...` from `backend/`.
- `db.Open` runs GORM `AutoMigrate` and seeds the default admin (`admin` / `admin123456`) on every boot, so `migrations/init.sql` is for manual MySQL setup only and is not required when starting from the Go server. If you add a model field, update the GORM struct; keep `init.sql` in sync only if you also maintain manual setups.
- On startup, `ReconcileInterruptedTasks` marks any `pending`/`running` tasks as `failed` ("task interrupted by server restart").
- Runtime/secret values (`jwt.secret`, `mcp.api_token`, `database.dsn`, deploy timeouts, `log_dir`) come only from `config.yaml` and need a restart. `PATCH /api/v1/settings` stores non-runtime metadata only.

### Deploy pipeline (internal/service)

Fixed stage order — deploys: `pull_code → unit_test → integration_test → build → deploy → health_check → notify`; rollbacks: `rollback → health_check → notify`. Empty commands are **skipped**, not failed. `pull_code` auto-generates `git fetch --all && git reset --hard origin/<branch>` when `pull_cmd` is empty.

- One `pending`/`running` task per project, enforced with `SELECT ... FOR UPDATE` on the project row → `409 ErrProjectBusy`.
- `runner.LocalCommandRunner` runs each command via `bash -lc` in its own process group (`Setpgid`); cancel/timeout kills the whole group. Per-task log at `{deploy.log_dir}/{task_id}.log`.

### Security constraints enforced in code

- Deploy/build/test commands come ONLY from project config — the frontend never passes command strings at runtime.
- `app_log_path` must be absolute and resolve under the project's `app_dir` (symlinks resolved when the target exists). Deploy-log reads are likewise confined under `deploy.log_dir`. See `safePathInDir` in `internal/handler/handler.go`.
- `sanitizeLogText` strips ANSI escapes and invalid UTF-8 from every tailed log and SSE log stream.
- SSE `/stream` endpoints accept `?access_token=<jwt or mcp token>` as a fallback because `EventSource` can't set headers.
- `webhook_secret` and `notify_webhook` are masked in API responses; a value containing `******` is treated as masked and ignored on PATCH (`isMaskedValue`).
- MCP mutation tools (`trigger_deploy`, `trigger_rollback`) are off unless `mcp.allow_mutation_tools=true` AND the tool call passes `confirm=true`.
- Webhook verification differs by provider: GitHub requires `X-Hub-Signature-256` HMAC-SHA256; Gitee accepts `X-Gitee-Token` / `X-Git-Osc-Token` header or `?token=` query.

## Frontend

```bash
npm install
npm run dev     # Vite; proxies /api → http://127.0.0.1:8088
npm run build   # tsc -b && vite build — this is the only type/compile check
```

- No test script and no lint script. `npm run build` is the only verification step.
- Mocks: `VITE_ENABLE_MOCKS=true npm run dev`. Only active in Vite dev AND only on fetch failure (catch path in `src/api/client.ts`); production builds never fake API responses.
- `components.json` (shadcn) declares `@/components` and `@/lib/utils` aliases, but those aliases are NOT configured in `tsconfig.json` or `vite.config.ts`. Existing UI components use relative imports (`../../lib/utils`). If you add shadcn components, either configure the alias or rewrite imports to relative — don't assume `@/` resolves.
