# Deployment Notes

Postdare Go targets Linux hosts with Git, systemd, and language-specific build tools installed by the user.

## Build Backend

```bash
cd backend
go build -o postdare-go-server ./cmd/server
```

Copy `postdare-go-server`, `config.yaml`, and migrations to `/opt/postdare-go/backend`.

Install the service:

```bash
sudo cp examples/postdare-go.service /etc/systemd/system/postdare-go.service
sudo systemctl daemon-reload
sudo systemctl enable --now postdare-go
```

## Deploy Stages

Each project defines an ordered list of deploy stages (`deploy_stages`). A stage is a
named shell command that runs in list order:

- `enabled: false` skips the stage.
- `continue_on_error: true` records the stage as failed but keeps the pipeline running.

After all stages complete, the fixed `health_check` (driven by `health_url`) and `notify`
steps run as before. Rollback stays separate and uses `rollback_cmd`.

Project commands should be explicit and absolute, for example:

```bash
cd /data/repos/my-app && git fetch --all && git reset --hard origin/main
cd /data/repos/my-app && mvn clean test
cd /data/repos/my-app && mvn package -DskipTests
bash /data/apps/my-app/deploy.sh
bash /data/apps/my-app/rollback.sh
```

Do not pass command strings from the frontend at deploy time. Store stages in the project
configuration.

## Logs

Deploy logs are written to:

```text
/data/postdare-go/logs/deploy/{task_id}.log
```

Application logs come from each project's configured `app_log_path`.

For safety, `app_log_path` must be an absolute path under the project's `app_dir`. Postdare Go rejects app log reads that escape that directory, including resolved symlinks when the target exists.
