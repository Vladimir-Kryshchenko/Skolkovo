# 🌐 Proxy Manager для «База Сколково»

## Бесплатные варианты обхода WAF

| Вариант | Лимит | Цена | IP | Сложность |
|---|---|---|---|---|
| **Cloudflare Workers** | 100K/день | Бесплатно | Резидентный | ⭐⭐ |
| **GitHub Actions** | 2000 мин/мес | Бесплатно | Дата-центр | ⭐⭐⭐ |
| **Webshare.io** | 10 прокси | Бесплатно | Дата-центр | ⭐ |
| **ProxyScrape** | Публичные листы | Бесплатно | Разные | ⭐⭐ |
| **IPRoyal** | Без лимита | от $3.5/GB | Резидентный | ⭐ |

## 🚀 Быстрый старт — Cloudflare Workers (рекомендуется)

### 1. Деплой Worker

```bash
# Установить Wrangler
npm install -g wrangler

# Авторизоваться
wrangler login

# Деплой
cd deploy/cloudflare-worker
wrangler deploy proxy.js --name skolkovo-proxy
```

### 2. Проверить

```bash
# Health check
curl https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/health

# Проверить IP
curl "https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=https://api.ipify.org"

# Скачивание документа
curl -o test.pdf "https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=https://dochub.sk.ru/foundation/documents/m/docs/24905.aspx"
```

### 3. Настроить в админке

Откройте `https://213.136.75.7/proxy` и:
1. Вставьте URL Cloudflare Worker
2. Нажмите "Сохранить"
3. Протестируйте кнопкой "🧪 Тест"

### 4. Запустить скачивание

```bash
ssh root@213.136.75.7 "docker exec baza-skolkovo-skolkovo-1 /app/skolkovo fetch"
```

## 📁 Структура файлов

```
deploy/
├── cloudflare-worker/
│   ├── proxy.js          # Cloudflare Worker код
│   ├── wrangler.json     # Wrangler конфиг
│   └── DEPLOY.md         # Инструкция по деплою
└── proxy-manager/
    ├── README.md         # Документация
    ├── .env.example      # Пример .env
    └── INTEGRATION.md    # Интеграция с кодом

src/admin/
├── proxy_manager.go      # API управление прокси
├── proxy_page.go         # UI страница прокси
└── proxy_client.go       # Клиент для скачивания

.github/workflows/
└── skolkovo-fetch.yml    # GitHub Actions workflow
```

## 🔧 Настройка в .env

```bash
# Cloudflare Worker
PROXY_TYPE=cloudflare
PROXY_CLOUDFLARE_URL=https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=

# Webshare.io
PROXY_TYPE=webshare
PROXY_WEBSHARE_URL=http://username:password@proxy.webshare.io:8080

# IPRoyal
PROXY_TYPE=iproyal
PROXY_IPROYAL_URL=http://username:password@royal-residential.com:PORT

# Без прокси
PROXY_TYPE=none
```

## 🧪 Тестирование

```bash
# Проверить IP без прокси
curl https://api.ipify.org

# С прокси
curl "https://skolkovo-proxy.YOUR-SUBDOMAIN.workers.dev/?url=https://api.ipify.org"

# Через админку
curl -u admin:password https://213.136.75.7/admin/api/proxy/test -X POST -d '{"type":"cloudflare"}'
```

## 📊 Статус прокси

| Прокси | Статус | IP | Последний тест |
|---|---|---|---|
| Cloudflare | ✅ Работает | 104.x.x.x | 2026-05-30 14:00 |
| Webshare | ❌ Не настроен | - | - |
| IPRoyal | ❌ Не настроен | - | - |

## ⚠️ Важные замечания

1. **Cloudflare Workers** — лучший бесплатный вариант, но лимит 100K запросов/день
2. **GitHub Actions** — работает, но IP меняется и может быть заблокирован
3. **Публичные прокси** — нестабильные, часто падают
4. **Резидентные прокси** (IPRoyal) — самый надёжный вариант, но платный

## 🔄 Автоматическое переключение

В админке можно настроить:
- Автоматическое переключение на резервный прокси при ошибке
- Тестирование всех прокси каждые 5 минут
- Уведомления в Telegram при смене прокси
