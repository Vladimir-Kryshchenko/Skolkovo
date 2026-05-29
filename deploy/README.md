# Развёртывание и запуск «База Сколково»

## 1. Поднять инфраструктуру

```bash
cp .env.example .env          # при необходимости измените MCP_API_KEY и пароли
docker compose -f deploy/docker-compose.yml up -d
```

Поднимутся три сервиса:

| Сервис | Порт | Назначение |
| :--- | :--- | :--- |
| Qdrant | 6333 (REST), 6334 (gRPC) | векторная БД |
| TEI | 8081 | эмбеддинги `multilingual-e5-base` |
| PostgreSQL | 5432 | реестр метаданных (опц., `STORE_BACKEND=postgres`) |

> Первый запуск TEI скачивает модель (~1–2 ГБ) — подождите, пока `/embed` станет доступен.
> Проверка: `skolkovo embed` (должно вернуть размерность эмбеддинга).

## 2. Собрать бинарь

```bash
go build -o bin/skolkovo ./cmd/skolkovo      # Windows: bin/skolkovo.exe
```

## 3. Команды

| Команда | Что делает |
| :--- | :--- |
| `skolkovo scrape` | каталог из RSS (~20 свежих) + обход HTML (E1) |
| `skolkovo catalog` | полное перечисление каталога по категориям через headless-браузер (нужна разрешённая сеть/прокси) |
| `skolkovo index [-force]` | проиндексировать действующие документы в Qdrant (E2) |
| `skolkovo fetch` | скачать тела файлов через headless-браузер (обход WAF; нужна разрешённая сеть/прокси) |
| `skolkovo news` | синхронизировать новости/RSS в RAG (E5) |
| `skolkovo sync` | полный цикл: документы + новости + индексация + отчёт |
| `skolkovo mcp` | открытый MCP-сервер (порт `MCP_ADDR`, по умолчанию `:8080`) |
| `skolkovo admin` | админка (порт `ADMIN_ADDR`, по умолчанию `:8090`) |
| `skolkovo serve` | всё сразу: планировщик + MCP + админка (продакшен-режим) |
| `skolkovo embed` | проверка доступности TEI |

> **Про источник:** список документов на dochub.sk.ru грузится JS-виджетом, поэтому
> каталог берётся из RSS-ленты. Страницы файлов `/m/docs/` закрыты анти-бот **WAF**
> (Variti): обычным клиентам и headless-браузеру с дата-центровых IP отдаётся 403.
> Документы с «УТРАТИЛИ СИЛУ» в заголовке автоматически получают статус «устарел».

### Полнота каталога

`scrape` (RSS) даёт только ~20 последних документов. Для **полного** каталога используйте
`skolkovo catalog` — он рендерит страницы категорий в headless-браузере (JS-виджет `superlist`
подгружает весь список) и заносит все документы с категориями. Дедуплицируется с RSS по ссылке.
Требует разрешённой сети/прокси (как и `fetch`).

### Как получить тела файлов (два пути)

1. **Ручная загрузка через админку (работает всегда).** В таблице у документа без
   файла есть поле загрузки (⬆): сотрудник, у которого документ открывается в браузере,
   скачивает его и заливает. Система сохраняет файл, считает хэш и (если статус
   «действует») индексирует.
2. **`skolkovo fetch` — headless-браузер (chromedp).** Открывает страницу документа в
   Chrome, исполняет JS-челлендж, извлекает ссылку и скачивает файл. **WAF блокирует
   дата-центровые IP**, поэтому запускать из разрешённой сети (рабочая машина) или через
   резидентный прокси: задайте `PROXY_URL` (и при необходимости `CHROME_PATH`, `FETCH_LIMIT`).

Типовой первый прогон:

```bash
skolkovo scrape          # скачать документы (статус «на_проверке»)
# открыть админку http://localhost:8090 и подтвердить нужные документы (статус «действует»)
skolkovo index           # проиндексировать подтверждённые
skolkovo serve           # запустить сервис целиком
```

## 4. Хранилище метаданных

- `STORE_BACKEND=json` (по умолчанию) — файл `Документы_Сколково/Метаданные/реестр_документов.json`, без инфраструктуры.
- `STORE_BACKEND=postgres` — таблица `documents` в PostgreSQL (схема применяется автоматически, см. `deploy/schema.sql`).

## 5. Подключение к открытому MCP-серверу

Эндпоинт: `http://<host>:8080/mcp` (Streamable HTTP). Авторизация — заголовком
`X-API-Key: <MCP_API_KEY>` или `Authorization: Bearer <MCP_API_KEY>`.

Проверка вручную:

```bash
# initialize
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "X-API-Key: $MCP_API_KEY" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"client","version":"1"}}}'

# поиск
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" \
  -H "X-API-Key: $MCP_API_KEY" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_documents","arguments":{"query":"статус резидента","limit":3}}}'
```

Подключение из Claude / агента (пример конфигурации MCP-клиента):

```json
{
  "mcpServers": {
    "baza-skolkovo": {
      "type": "streamable-http",
      "url": "http://<host>:8080/mcp",
      "headers": { "X-API-Key": "<MCP_API_KEY>" }
    }
  }
}
```

Инструменты MCP: `search_documents`, `get_document`, `list_active_documents`.
