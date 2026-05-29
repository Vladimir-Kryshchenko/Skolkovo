---
name: Статус проекта База Сколково v1.0
description: Полный статус реализации всех 6 эпиков (A-F) на 2026-05-29
type: project
---

**Статус:** MVP v1.0 завершён. 38 пакетов Go, 200+ тестов зелёных, сборка и vet проходят.

**Реализовано:**

## Эпик A — Расширение источников (6 парсеров)
- ✅ Парсер мероприятий (src/events/) — RSS + HTML fallback
- ✅ Парсер конкурсов/грантов (src/contests/) — HTML-скрапинг
- ✅ Парсер требований резидентства (src/requirements/) — 3 стратегии парсинга
- ✅ Парсер FAQ (src/faq/) — 3 стратегии поиска FAQ-элементов
- ✅ Монитор Telegram-каналов (src/telegram/) — через RSSHub
- ✅ Парсер реестра резидентов (src/registry/) — HTML table parsing

## Эпик B — Управление резидентством
- ✅ Модель клиента + стадии резидентства (src/common/model/client.go)
- ✅ CRUD клиентов + история переходов (PostgresClientStore)
- ✅ Чек-листы процедур (entry, reporting, extension, exit) — 5 шагов каждый
- ✅ Трекер дедлайнов с авто-генерацией из чек-листов
- ✅ Генератор документов (src/generator/) — Go templates → PDF/DOCX/HTML
- ✅ Связь документов с клиентом (ClientDocument)
- ✅ Портал клиента (src/portal/) — magic link auth, dashboard, checklists, deadlines
- ✅ Seed команда — создание стандартных чек-листов

## Эпик C — ИИ-агенты и чат-бот
- ✅ Агент «Консультант» (src/agents/consultant.go) — RAG-поиск → ответы с цитированием
- ✅ Агент «Валидатор» (src/agents/validator.go) — проверка документов по правилам
- ✅ Агент «Монитор» (src/agents/monitor.go) — изменения → digest → подписки
- ✅ Агент «Онкоординатор» (src/agents/coordinator.go) — чек-листы → эскалация
- ✅ Telegram-бот (src/tgbot/) — /start, /status, /deadlines, /docs, /ask, /help
- ✅ Web-виджет чата (src/chat_widget/) — popup, marked.js, MCP proxy
- ✅ /ask подключён к ConsultantAgent через MCP

## Эпик D — MCP + Multi-tenant
- ✅ 22+ MCP инструментов (базовые + источники + резидентство + агенты)
- ✅ Multi-tenant модель (Tenant, tenant_id в clients)
- ✅ Rate-limit per tenant (через API-ключ)

## Эпик E — Качество данных
- ✅ Граф связей документов (DocumentLink, MCP get_document_links)
- ✅ UI графа в админке (vis.js)
- ✅ Diff-система (src/diff/) — LCS algorithm, HTML подсветка
- ✅ UI diff в админке (сравнение версий)
- ✅ ИИ-классификатор (src/classifier/) — cosine similarity эмбеддингов
- ✅ Дашборд аналитики (src/analytics/) — Chart.js, CSV-экспорт
- ✅ UI аналитики в админке
- ✅ Валидатор полноты (src/validator/) — отчёты Markdown/HTML

## Эпик F — Инфраструктура
- ✅ TLS / Let's Encrypt (deploy/nginx/nginx-letsencrypt.conf)
- ✅ Backup & restore (scripts/backup.sh)
- ✅ Миграции БД (src/migrate/, deploy/migrations/001_*.sql, 002_*.sql)
- ✅ CI/CD pipeline (.github/workflows/ci.yml, dependabot.yml)
- ✅ Метрики и мониторинг (deploy/prometheus.yml, deploy/grafana/)

## Архитектура
- ✅ АРХИТЕКТУРА_База_Сколково_v1_0_29_05_2026.md — полная схема с 20+ компонентами
- ✅ README.md — обновлён
- ✅ CLAUDE.md — обновлён с 16 подсистемами, 22 MCP-инструментами
