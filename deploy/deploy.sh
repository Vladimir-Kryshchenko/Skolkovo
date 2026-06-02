#!/usr/bin/env bash
# Разворот «База Сколково» на сервере: .env с секретами → сертификат → сборка/запуск → cron-бэкап.
# Идемпотентно: повторный запуск не перезатирает .env и сертификат.
set -euo pipefail

SERVER_IP="${SERVER_IP:-213.136.75.7}"
PROJECT_DIR="${PROJECT_DIR:-/opt/baza-skolkovo}"
DEPLOY_DIR="$PROJECT_DIR/deploy"
cd "$DEPLOY_DIR"

# 1. Секреты (.env) — генерируем один раз
if [ ! -f .env ]; then
  echo "[env] генерирую deploy/.env с секретами"
  MCP_API_KEY="$(openssl rand -hex 24)"
  ADMIN_PASSWORD="$(openssl rand -hex 16)"
  POSTGRES_PASSWORD="$(openssl rand -hex 16)"
  cat > .env <<EOF
# Сгенерировано deploy.sh — секреты тестового сервера.
POSTGRES_USER=skolkovo
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_DB=skolkovo
QDRANT_COLLECTION=skolkovo_docs
EMBEDDING_DIM=768
STORE_BACKEND=postgres
MCP_API_KEY=$MCP_API_KEY
MCP_RATE_LIMIT_RPS=5
ADMIN_USER=admin
ADMIN_PASSWORD=$ADMIN_PASSWORD
NEWS_RSS_URL=https://sk.ru/news/rss/
NOTIFY_WEBHOOK=
# Обход WAF dochub для скачивания тел файлов (/m/docs/ за Variti).
# С зарубежного датацентрового IP сервера тела файлов НЕ качаются (403/timeout) —
# нужен рабочий РЕЗИДЕНТНЫЙ RU-прокси. Задайте ОДИН из вариантов и пересоздайте
# контейнер: docker compose -f docker-compose.prod.yml --env-file .env up -d
#   PROXY_URL      — резидентный RU-прокси: http://user:pass@host:port
#   PROXY6_API_KEY — ключ proxy6.net (автоподбор RU-прокси при WAF-бане)
CHROME_PATH=/usr/bin/chromium
PROXY_URL=
PROXY6_API_KEY=
EOF
  chmod 600 .env
  echo "=================== СЕКРЕТЫ (сохраните) ==================="
  echo " MCP_API_KEY       = $MCP_API_KEY"
  echo " ADMIN_USER        = admin"
  echo " ADMIN_PASSWORD    = $ADMIN_PASSWORD"
  echo " POSTGRES_PASSWORD = $POSTGRES_PASSWORD"
  echo "=========================================================="
else
  echo "[env] deploy/.env уже существует — не трогаю"
fi

# 2. Self-signed сертификат
bash "$DEPLOY_DIR/gen-cert.sh" "$SERVER_IP"

# 3. Сборка и запуск стека
echo "[compose] сборка и запуск (build может занять несколько минут)"
docker compose -f docker-compose.prod.yml --env-file .env up -d --build

# 4. Cron на ежедневный бэкап (03:30), хранение 30 дней (ротация — в backup.sh)
echo "[cron] установка ежедневного бэкапа"
cp "$PROJECT_DIR/scripts/backup.sh" /usr/local/bin/skolkovo-backup.sh
chmod +x /usr/local/bin/skolkovo-backup.sh
cat > /etc/cron.d/baza-skolkovo-backup <<'EOF'
# Ежедневный бэкап «База Сколково» в 03:30. Ротация (30 дней) — внутри скрипта.
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
30 3 * * * root /usr/local/bin/skolkovo-backup.sh >> /var/log/baza-skolkovo-backup.log 2>&1
EOF
chmod 644 /etc/cron.d/baza-skolkovo-backup
systemctl enable --now cron >/dev/null 2>&1 || service cron restart >/dev/null 2>&1 || true

echo "[done] разворот завершён. Стек:"
docker compose -f docker-compose.prod.yml ps
