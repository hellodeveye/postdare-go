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

## Version

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/version` | Build version and Go runtime version |

## Auth

| Method | Path | Description |
| --- | --- | --- |
| POST | `/api/v1/auth/login` | Login |
| GET | `/api/v1/auth/me` | Current user |
| POST | `/api/v1/auth/logout` | Logout |
| PUT | `/api/v1/auth/password` | Change current user's password |

`POST /auth/login` and `GET /auth/me` include `must_change_password`. When it is `true`, secured endpoints except `GET /auth/me`, `POST /auth/logout`, and `PUT /auth/password` return:

```json
{
  "error": {
    "code": "PASSWORD_CHANGE_REQUIRED",
    "message": "Password change is required before continuing"
  }
}
```

Change password request:

```json
{
  "old_password": "current-password",
  "new_password": "new-password"
}
```

The new password must be at least 8 characters.

## Projects

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/projects` | List projects |
| POST | `/api/v1/projects` | Create project |
| GET | `/api/v1/projects/{project_id}` | Get project |
| PATCH | `/api/v1/projects/{project_id}` | Update project |
| DELETE | `/api/v1/projects/{project_id}` | Delete project and related database records |
| POST | `/api/v1/projects/{project_id}/deploy-tasks` | Trigger deploy, returns `202 Accepted` |
| POST | `/api/v1/projects/{project_id}/rollback-tasks` | Trigger rollback, returns `202 Accepted` |
| GET | `/api/v1/projects/{project_id}/app-logs` | Read app log tail |
| GET | `/api/v1/projects/{project_id}/app-logs/stream` | Stream app log over SSE |

Project deletion is destructive for database records: it removes the project, related deploy tasks, related deploy task stages, and webhook events associated with the project. It returns `409 Conflict` if the project has a `pending` or `running` deploy or rollback task. Physical deploy log files and application log files are not removed.

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

`PATCH /settings` does not change live runtime configuration such as `jwt.secret`, `mcp.api_token`, deploy timeouts, or database settings. Runtime and secret settings are managed by environment variables, optional `config.yaml`, and generated `secrets.yaml`; changes require a backend restart.
