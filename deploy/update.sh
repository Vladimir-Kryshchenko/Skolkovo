#!/usr/bin/env bash
# Обновление «База Сколково» на сервере из git.
#
# Предполагается, что /opt/baza-skolkovo — git-чекаут ветки main. Секреты (deploy/.env),
# TLS-сертификаты (deploy/certs/) и данные (data/, docker volumes) НЕ в git и сохраняются.
#
# Использование:  bash deploy/update.sh           (пересобрать только skolkovo)
#                 bash deploy/update.sh --all      (пересобрать весь стек)
set -euo pipefail

PROJECT_DIR="${PROJECT_DIR:-/opt/baza-skolkovo}"
cd "$PROJECT_DIR"

echo "[update] git pull --ff-only origin main"
git pull --ff-only origin main

cd "$PROJECT_DIR/deploy"
if [ "${1:-}" = "--all" ]; then
  echo "[update] пересборка и перезапуск всего стека"
  docker compose -f docker-compose.prod.yml --env-file .env up -d --build
else
  echo "[update] пересборка и перезапуск сервиса skolkovo"
  docker compose -f docker-compose.prod.yml --env-file .env up -d --build skolkovo
fi

echo "[update] готово. Состояние:"
docker compose -f docker-compose.prod.yml ps
