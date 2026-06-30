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

## Application Scripts

Project commands should be explicit and absolute:

```bash
cd /data/repos/my-app && mvn clean test
cd /data/repos/my-app && mvn package -DskipTests
bash /data/apps/my-app/deploy.sh
bash /data/apps/my-app/rollback.sh
```

Do not pass command strings from the frontend at deploy time. Store commands in the project configuration.

## Logs

Deploy logs are written to:

```text
/data/postdare-go/logs/deploy/{task_id}.log
```

Application logs come from each project's configured `app_log_path`.

For safety, `app_log_path` must be an absolute path under the project's `app_dir`. Postdare Go rejects app log reads that escape that directory, including resolved symlinks when the target exists.
