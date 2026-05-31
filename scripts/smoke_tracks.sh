#!/usr/bin/env bash
# Smoke-проверка треков «Актуальность изменений», «Единый индекс», «Консультант», «Файлы».
# Запускать на сервере, где поднят `skolkovo serve` (Qdrant+TEI+Postgres).
#
# Использование:
#   MCP=http://localhost:8080 KEY=<MCP_API_KEY> PORTAL=http://localhost:8092 bash scripts/smoke_tracks.sh
#
# Скрипт только читает данные (никаких мутаций) и печатает результаты.
set -u

MCP="${MCP:-http://localhost:8080}"
KEY="${KEY:-}"
PORTAL="${PORTAL:-http://localhost:8092}"
AUTH=(); [ -n "$KEY" ] && AUTH=(-H "Authorization: Bearer $KEY")

call() { # call <tool> <args-json>
  curl -s "${AUTH[@]}" -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" \
    "$MCP/mcp"
}

echo "== 1. Единый индекс: семантический поиск (документы+события+конкурсы+FAQ) =="
call search_documents '{"query":"гранты и конкурсы для резидентов","limit":5}'
echo; echo "   (проверьте, что в результатах есть entity_type: event/contest/faq, не только document)"

echo; echo "== 2. Актуальность: критичные изменения за 30 дней =="
call get_recent_changes '{"since_days":30,"min_severity":"warning","limit":10}'
echo; echo "   (поля severity / analysis_summary / affected_stages должны быть заполнены)"

echo; echo "== 3. Консультант (RAG+LLM, ответ с источниками) =="
call ask_consultant '{"question":"Какие требования к отчётности резидента Сколково?"}'
echo; echo "   (ожидается связный ответ + блок 📚 Источники)"

echo; echo "== 4. Скачивание файла документа через MCP (нужен реальный id) =="
DOCID="$(call list_active_documents '{}' | grep -o '"id":"[^"]*"' | head -1 | cut -d'\"' -f4)"
if [ -n "$DOCID" ]; then
  echo "   doc id: $DOCID"
  call get_document_file "{\"id\":\"$DOCID\"}" | head -c 400
  echo; echo "   (ожидается filename/mime/size_bytes и base64 либо note про size/source_url)"
else
  echo "   нет действующих документов — пропуск"
fi

echo; echo "== 5. Здоровье источников =="
call get_source_health '{}'

echo; echo "== Готово. Портал inbox: $PORTAL/notifications (после входа по magic-link)."
echo "   Дашборд консультанта: блок «Важные изменения» на :8094/consultant/dashboard"
