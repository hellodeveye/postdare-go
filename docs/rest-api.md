# REST API

All management endpoints use JWT or the configured MCP API token:

```http
Authorization: Bearer TOKEN
```

Successful response:

```json
{
  "data": {},
  "request_id": "req_xxx"
}
```

List response:

```json
{
  "data": [],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 100
  },
  "request_id": "req_xxx"
}
```

Error response:

```json
{
  "error": {
    "code": "PROJECT_NOT_FOUND",
    "message": "Project not found",
    "details": {}
  },
  "request_id": "req_xxx"
}
```

## Auth

| Method | Path | Description |
| --- | --- | --- |
| POST | `/api/v1/auth/login` | Login |
| GET | `/api/v1/auth/me` | Current user |
| POST | `/api/v1/auth/logout` | Logout |

## Projects

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/projects` | List projects |
| POST | `/api/v1/projects` | Create project |
| GET | `/api/v1/projects/{project_id}` | Get project |
| PATCH | `/api/v1/projects/{project_id}` | Update project |
| DELETE | `/api/v1/projects/{project_id}` | Delete project metadata |
| POST | `/api/v1/projects/{project_id}/deploy-tasks` | Trigger deploy, returns `202 Accepted` |
| POST | `/api/v1/projects/{project_id}/rollback-tasks` | Trigger rollback, returns `202 Accepted` |
| GET | `/api/v1/projects/{project_id}/app-logs` | Read app log tail |
| GET | `/api/v1/projects/{project_id}/app-logs/stream` | Stream app log over SSE |

## Deploy Tasks

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/deploy-tasks` | List tasks |
| GET | `/api/v1/deploy-tasks/{task_id}` | Task detail |
| GET | `/api/v1/deploy-tasks/{task_id}/stages` | Task stages |
| GET | `/api/v1/deploy-tasks/{task_id}/logs` | Deploy log tail |
| GET | `/api/v1/deploy-tasks/{task_id}/logs/stream` | Stream deploy log over SSE |
| POST | `/api/v1/deploy-tasks/{task_id}/cancel` | Mark pending/running task canceled |
| GET | `/api/v1/deploy-tasks/{task_id}/analysis` | Rule-based failure analysis |

## Webhook Events

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/webhook-events` | List webhook deliveries |
| GET | `/api/v1/webhook-events/{event_id}` | Delivery detail |

## Webhook Callback

| Method | Path | Description |
| --- | --- | --- |
| POST | `/api/v1/webhooks/gitee/{project_key}` | Gitee callback |
| POST | `/api/v1/webhooks/github/{project_key}` | GitHub callback |

## Dashboard and Settings

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/dashboard/summary` | Summary metrics |
| GET | `/api/v1/dashboard/recent-deploy-tasks` | Recent deployments |
| GET | `/api/v1/settings` | Runtime settings |
| PATCH | `/api/v1/settings` | Save non-runtime metadata settings |

Lists support `page`, `page_size`, `sort`, and resource-specific filters such as `project_id`, `status`, and `provider`.

`PATCH /settings` does not change live runtime configuration such as `jwt.secret`, `mcp.api_token`, deploy timeouts, or database DSN. Runtime and secret settings are managed in `config.yaml` and require a backend restart.
