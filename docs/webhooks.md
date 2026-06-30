# Webhook Configuration

Postdare Go exposes provider-specific callback URLs. Webhook endpoints are public, but each request must pass the configured token or signature check.

## Gitee

Callback URL:

```text
http://YOUR_HOST:8088/api/v1/webhooks/gitee/PROJECT_KEY?token=WEBHOOK_SECRET
```

Project requirements:

- `git_provider` is `gitee`
- `webhook_secret` matches the `token` query value or Gitee token header
- `auto_deploy_enabled` is true
- The push branch matches the project `branch`

Postdare Go reads common Gitee fields:

- `ref`
- `after`
- `head_commit.id`
- `head_commit.message`
- `head_commit.author.name`
- `commits`

Branch mismatch, disabled auto deploy, invalid token, and unsupported events are saved in `webhook_events` with `ignored_reason`.

## GitHub

Callback URL:

```text
http://YOUR_HOST:8088/api/v1/webhooks/github/PROJECT_KEY
```

Set the GitHub webhook secret to the project's `webhook_secret`.

Postdare Go verifies:

```text
X-Hub-Signature-256 = sha256=HMAC_SHA256(webhook_secret, raw_body)
```

The comparison uses constant-time HMAC comparison. Postdare Go also reads:

- `X-GitHub-Event`
- `X-GitHub-Delivery`
- `ref`
- `after`
- `head_commit.message`
- `head_commit.author.name`

Only `push` events are deployed.
