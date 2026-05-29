# Быстрая инструкция по MCP серверу

## Текущий статус
✅ MCP сервер настроен и работает на удалённом сервере

## Конфигурация
- **API ключ:** `517a4b18d8701532ce5e9d50671395b8602a9f9e68691f1d`
- **Endpoint:** `https://213.136.75.7/mcp`
- **Конфиг Qwen Code:** `.qwen/settings.json`

## Доступные инструменты
1. **search_documents** — семантический поиск по документам
2. **get_document** — получение метаданных документа по ID
3. **list_active_documents** — список действующих документов

## Проверка работоспособности
```bash
# Health check (с флагом -k для self-signed SSL)
curl -sk https://213.136.75.7/health

# MCP initialize
curl -sk -X POST https://213.136.75.7/mcp ^
  -H "Content-Type: application/json" ^
  -H "X-API-Key: 517a4b18d8701532ce5e9d50671395b8602a9f9e68691f1d" ^
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"clientInfo\":{\"name\":\"test\",\"version\":\"1\"}}}"
```

## Для полноценного поиска
На удалённом сервере должна быть запущена инфраструктура:
- Qdrant (векторная БД)
- TEI (сервис эмбеддингов)
- PostgreSQL (реестр метаданных)
