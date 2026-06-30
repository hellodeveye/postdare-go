#!/usr/bin/env bash
set -euo pipefail

APP_NAME="my-app"
APP_DIR="/data/apps/${APP_NAME}"
BACKUP_DIR="${APP_DIR}/backups"
JAR_TARGET="${APP_DIR}/${APP_NAME}.jar"

LATEST_BACKUP="$(ls -t "${BACKUP_DIR}"/${APP_NAME}-*.jar | head -n 1)"

if [[ -z "${LATEST_BACKUP}" ]]; then
  echo "no backup artifact found"
  exit 1
fi

cp "${LATEST_BACKUP}" "${JAR_TARGET}"
systemctl restart "${APP_NAME}"
systemctl --no-pager status "${APP_NAME}"
