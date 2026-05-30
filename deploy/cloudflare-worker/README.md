# 🚀 Инструкция по деплою Cloudflare Worker

## Что нужно

1. **Cloudflare аккаунт** (бесплатно) → https://dash.cloudflare.com
2. **API Token** с правами Workers
3. **Account ID** из Cloudflare dashboard

## Шаг 1: Создать API Token

1. Перейдите на https://dash.cloudflare.com/profile/api-tokens
2. Нажмите **"Create Token"**
3. Выберите шаблон **"Edit Cloudflare Workers"**
4. В разделе **"Account Resources"** выберите ваш аккаунт
5. Нажмите **"Continue to summary"** → **"Create Token"**
6. **Скопируйте токен** (он показывается только один раз!)

## Шаг 2: Найти Account ID

1. Перейдите на https://dash.cloudflare.com
2. Account ID находится в правом нижнем углу dashboard
3. Или перейдите на https://dash.cloudflare.com/profile → справа будет "Account ID"

## Шаг 3: Деплой Worker

На сервере 213.136.75.7 выполните:

```bash
# Замените YOUR_API_TOKEN и YOUR_ACCOUNT_ID на реальные значения
bash /tmp/deploy_cf_worker.sh YOUR_API_TOKEN YOUR_ACCOUNT_ID
```

Или используйте wrangler (если предпочитаете):

```bash
# Авторизоваться
wrangler login

# Деплой
cd /tmp
wrangler deploy proxy.js --name skolkovo-proxy
```

## Шаг 4: Проверить Worker

```bash
# Health check
curl https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/health

# Проверить IP (должен отличаться от 213.136.75.7)
curl "https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=https://api.ipify.org"

# Скачать тестовый документ
curl -o test.pdf "https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=https://dochub.sk.ru/foundation/documents/m/docs/24905.aspx"
```

## Шаг 5: Настроить в .env

```bash
# Добавить в .env:
echo 'PROXY_TYPE=cloudflare' >> /opt/baza-skolkovo/deploy/.env
echo 'PROXY_CLOUDFLARE_URL=https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=' >> /opt/baza-skolkovo/deploy/.env

# Перезапустить контейнер:
cd /opt/baza-skolkovo
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build skolkovo
```

## Шаг 6: Запустить скачивание

```bash
# Запустить скачивание документов:
docker exec baza-skolkovo-skolkovo-1 /app/skolkovo fetch

# Проверить результаты:
docker exec baza-skolkovo-postgres-1 psql -U skolkovo -d skolkovo -c 'SELECT local_path IS NOT NULL as has_file, count(*) FROM documents GROUP BY 1;'
```

## Troubleshooting

### Worker не отвечает
```bash
# Проверить статус
curl https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/health

# Если ошибка 500 - проверьте код в Cloudflare dashboard
# Перейдите в Workers & Pages → skolkovo-proxy → Logs
```

### Документы не скачиваются
```bash
# Проверить логи контейнера
docker logs baza-skolkovo-skolkovo-1 --tail 50

# Проверить прокси напрямую
curl "https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=https://dochub.sk.ru/foundation/documents/m/docs/24905.aspx" -o test.pdf
```

### Proxy не работает
```bash
# Проверить env vars в контейнере
docker exec baza-skolkovo-skolkovo-1 sh -c 'echo PROXY_TYPE=$PROXY_TYPE && echo PROXY_CLOUDFLARE_URL=$PROXY_CLOUDFLARE_URL'

# Перезапустить контейнер
docker compose -f deploy/docker-compose.yml --env-file deploy/.env down skolkovo
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build skolkovo
```
