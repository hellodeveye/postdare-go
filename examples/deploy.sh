#!/usr/bin/env bash
set -euo pipefail

APP_NAME="my-app"
APP_DIR="/data/apps/${APP_NAME}"
REPO_DIR="/data/repos/${APP_NAME}"
JAR_SOURCE="${REPO_DIR}/target/${APP_NAME}.jar"
JAR_TARGET="${APP_DIR}/${APP_NAME}.jar"
BACKUP_DIR="${APP_DIR}/backups"

mkdir -p "${APP_DIR}" "${BACKUP_DIR}"

if [[ -f "${JAR_TARGET}" ]]; then
  cp "${JAR_TARGET}" "${BACKUP_DIR}/${APP_NAME}-$(date +%Y%m%d%H%M%S).jar"
fi

cp "${JAR_SOURCE}" "${JAR_TARGET}"
systemctl restart "${APP_NAME}"
systemctl --no-pager status "${APP_NAME}"
