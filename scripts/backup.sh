#!/usr/bin/env bash
# Ежедневный бэкап «База Сколково»: Postgres + тома (Qdrant, docs, reports) + .env.
# Ротация: файлы старше RETENTION_DAYS дней удаляются.
set -euo pipefail

DEPLOY_DIR="${DEPLOY_DIR:-/opt/baza-skolkovo/deploy}"
DEST="${BACKUP_DEST:-/opt/backups/baza-skolkovo}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
PROJECT="baza-skolkovo"   # имя docker-проекта (compose `name:`) — префикс томов

COMPOSE="docker compose -f $DEPLOY_DIR/docker-compose.prod.yml --env-file $DEPLOY_DIR/.env"

# Учётки Postgres из .env
set -a; . "$DEPLOY_DIR/.env"; set +a

STAMP="$(date +%Y-%m-%d_%H%M)"
mkdir -p "$DEST"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "[$(date '+%F %T')] backup start -> $DEST/skolkovo-backup-$STAMP.tar.gz"

# 1. Postgres — логический дамп
$COMPOSE exec -T postgres pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" | gzip > "$TMP/postgres.sql.gz"

# 2. Тома: Qdrant (вектора), docs (первоисточники), reports (отчёты парсинга)
for vol in qdrant_data docs_data reports_data; do
  docker run --rm -v "${PROJECT}_${vol}:/v:ro" -v "$TMP:/out" alpine \
    tar czf "/out/${vol}.tar.gz" -C /v . 2>/dev/null
done

# 3. Настройки
cp "$DEPLOY_DIR/.env" "$TMP/env.backup"

# Единый архив
tar czf "$DEST/skolkovo-backup-$STAMP.tar.gz" -C "$TMP" .
echo "[$(date '+%F %T')] backup done: $(du -h "$DEST/skolkovo-backup-$STAMP.tar.gz" | cut -f1)"

# 4. Ротация — удалить бэкапы старше RETENTION_DAYS дней
echo "[$(date '+%F %T')] ротация (> ${RETENTION_DAYS} дн.):"
find "$DEST" -name 'skolkovo-backup-*.tar.gz' -type f -mtime +"$RETENTION_DAYS" -print -delete
