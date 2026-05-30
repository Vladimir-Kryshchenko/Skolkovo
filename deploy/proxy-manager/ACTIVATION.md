# Инструкция по активации прокси

## 1. Cloudflare Workers (бесплатно, 100K запросов/день)

### Шаг 1: Создать аккаунт Cloudflare
1. Перейдите на https://dash.cloudflare.com
2. Зарегистрируйтесь (бесплатно)
3. Подтвердите email

### Шаг 2: Установить Wrangler CLI
```bash
# На сервере 213.136.75.7 (уже установлено):
npm install -g wrangler

# Или локально на вашем компьютере:
npm install -g wrangler
```

### Шаг 3: Авторизоваться
```bash
wrangler login
```
Откроется браузер → авторизуйтесь в Cloudflare.

### Шаг 4: Деплой Worker
```bash
# Скопируйте файл на сервер:
scp deploy/cloudflare-worker/proxy.js root@213.136.75.7:/tmp/proxy.js
scp deploy/cloudflare-worker/wrangler.json root@213.136.75.7:/tmp/wrangler.json

# На сервере:
ssh root@213.136.75.7
cd /tmp
wrangler deploy proxy.js --name skolkovo-proxy
```

Получите URL вида:
```
https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev
```

### Шаг 5: Проверить
```bash
# Health check
curl https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/health

# Проверить IP
curl "https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=https://api.ipify.org"

# Скачивание документа
curl -o test.pdf "https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=https://dochub.sk.ru/foundation/documents/m/docs/24905.aspx"
```

### Шаг 6: Настроить в .env
```bash
# На сервере 213.136.75.7:
echo 'PROXY_TYPE=cloudflare' >> /opt/baza-skolkovo/deploy/.env
echo 'PROXY_CLOUDFLARE_URL=https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=' >> /opt/baza-skolkovo/deploy/.env

# Перезапустить:
cd /opt/baza-skolkovo
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build skolkovo

# Запустить скачивание:
docker exec baza-skolkovo-skolkovo-1 /app/skolkovo fetch
```

## 2. GitHub Actions (бесплатно, 2000 мин/мес)

### Шаг 1: Добавить secrets в GitHub repo
```bash
# В директории проекта:
gh auth login

# Добавить secrets:
gh secret set POSTGRES_DSN --body "postgres://skolkovo:skolkovo@213.136.75.7:5432/skolkovo?sslmode=disable"
gh secret set QDRANT_URL --body "http://213.136.75.7:6333"
gh secret set TEI_URL --body "http://213.136.75.7:8081"

# Добавить SSH ключ для загрузки файлов:
gh secret set SERVER_HOST --body "213.136.75.7"
gh secret set SERVER_USER --body "root"
gh secret set SSH_PRIVATE_KEY --body "$(cat ~/.ssh/id_ed25519)"
```

### Шаг 2: Запустить workflow
```bash
gh workflow run skolkovo-fetch.yml
```

Workflow запустится автоматически каждые 6 часов.

## 3. IPRoyal (платный, от $3.5/GB)

### Шаг 1: Регистрация
1. Перейдите на https://iproyal.com
2. Зарегистрируйтесь
3. Купите резидентные прокси

### Шаг 2: Настроить в .env
```bash
echo 'PROXY_TYPE=iproyal' >> /opt/baza-skolkovo/deploy/.env
echo 'PROXY_IPROYAL_URL=http://username:password@proxy-server:port' >> /opt/baza-skolkovo/deploy/.env

# Перезапустить:
cd /opt/baza-skolkovo
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build skolkovo
```

## 4. Тестирование прокси

```bash
# Проверить текущий прокси:
ssh root@213.136.75.7 "docker exec baza-skolkovo-skolkovo-1 sh -c 'echo PROXY_TYPE=\$PROXY_TYPE && echo PROXY_URL=\$PROXY_CLOUDFLARE_URL'"

# Протестировать скачивание:
ssh root@213.136.75.7 "docker exec baza-skolkovo-skolkovo-1 /app/skolkovo fetch"

# Проверить результаты:
ssh root@213.136.75.7 "docker exec baza-skolkovo-postgres-1 psql -U skolkovo -d skolkovo -c 'SELECT status, count(*) FROM documents GROUP BY status;'"
```

## 5. Мониторинг

```bash
# Проверить логи:
ssh root@213.136.75.7 "docker logs baza-skolkovo-skolkovo-1 --tail 50"

# Проверить скачанные файлы:
ssh root@213.136.75.7 "docker exec baza-skolkovo-skolkovo-1 ls -la /data/docs/На_проверке/Загружено/"

# Проверить статус документов:
ssh root@213.136.75.7 "docker exec baza-skolkovo-postgres-1 psql -U skolkovo -d skolkovo -c 'SELECT local_path IS NOT NULL as has_file, count(*) FROM documents GROUP BY 1;'"
```
