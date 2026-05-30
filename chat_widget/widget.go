// Package widget реализует HTTP-сервер веб-виджета чата с MCP-бэкендом.
package widget

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WidgetConfig хранит параметры виджета.
type WidgetConfig struct {
	// MCPURL — адрес MCP-сервера (Streamable HTTP), например http://localhost:8080.
	MCPURL string
	// MCPAPIKey — API-ключ для авторизации на MCP-сервере.
	MCPAPIKey string
	// PrimaryColor — основной цвет виджета (CSS-цвет), по умолчанию "#6366f1".
	PrimaryColor string
	// LogoURL — URL логотипа в шапке чата.
	LogoURL string
	// WelcomeMessage — приветственное сообщение.
	WelcomeMessage string
	// ListenAddr — адрес HTTP-сервера, по умолчанию ":8090".
	ListenAddr string
}

// WidgetServer — HTTP-сервер, отдающий JS-виджет и обрабатывающий запросы чата.
type WidgetServer struct {
	config       WidgetConfig
	sessionStore *SessionStore
	server       *http.Server
}

// Session описывает сессию чата.
type Session struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
}

// SessionStore хранит сессии в памяти.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionStore создаёт хранилище сессий.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

// GetOrCreate возвращает существующую сессию или создаёт новую.
func (s *SessionStore) GetOrCreate(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id != "" {
		if sess, ok := s.sessions[id]; ok {
			return sess
		}
	}

	sess := &Session{
		ID:        uuid.New().String(),
		CreatedAt: 0,
	}
	s.sessions[sess.ID] = sess
	return sess
}

// NewWidgetServer создаёт сервер виджета с заданной конфигурацией.
func NewWidgetServer(config WidgetConfig) *WidgetServer {
	if config.PrimaryColor == "" {
		config.PrimaryColor = "#6366f1"
	}
	if config.WelcomeMessage == "" {
		config.WelcomeMessage = "Здравствуйте! Чем могу помочь?"
	}
	if config.ListenAddr == "" {
		config.ListenAddr = ":8090"
	}

	return &WidgetServer{
		config:       config,
		sessionStore: NewSessionStore(),
	}
}

// Start запускает HTTP-сервер виджета. Блокирует до отмены контекста.
func (s *WidgetServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/chat-widget.js", s.serveWidgetJS)
	mux.HandleFunc("/chat", s.serveChatPage)
	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/chat", s.handleChat)

	s.server = &http.Server{
		Addr:    s.config.ListenAddr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("[widget] сервер запущен на %s", s.config.ListenAddr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// chatPageHTML — шаблон standalone-страницы чата.
var chatPageHTML = template.Must(template.New("chat").Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Чат — Сколково</title>
<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
<style>
:root {
  --primary: {{.PrimaryColor}};
  --bg: #f3f4f6; --surface: #fff; --msg-bg: #f3f4f6; --msg-code: #e5e7eb;
  --text: #1f2937; --text-muted: #9ca3af; --border: #e5e7eb; --input-border: #d1d5db;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #0f172a; --surface: #1e293b; --msg-bg: #334155; --msg-code: #475569;
    --text: #e2e8f0; --text-muted: #94a3b8; --border: #334155; --input-border: #475569;
  }
}
:root[data-theme="dark"] {
  --bg: #0f172a; --surface: #1e293b; --msg-bg: #334155; --msg-code: #475569;
  --text: #e2e8f0; --text-muted: #94a3b8; --border: #334155; --input-border: #475569;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); display: flex; justify-content: center; align-items: center; min-height: 100vh; }
.chat-container { width: 100%; max-width: 520px; background: var(--surface); border-radius: 16px; box-shadow: 0 4px 24px rgba(0,0,0,.12); display: flex; flex-direction: column; height: 80vh; overflow: hidden; }
.chat-header { background: var(--primary); color: #fff; padding: 16px 20px; display: flex; align-items: center; gap: 12px; }
.chat-header img { height: 32px; border-radius: 4px; }
.chat-header h2 { font-size: 18px; font-weight: 600; flex: 1; }
.chat-header-btn { background: rgba(255,255,255,.2); color: #fff; border: 1px solid rgba(255,255,255,.3); border-radius: 6px; font-size: 14px; padding: 4px 8px; cursor: pointer; }
.chat-messages { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 12px; }
.message { max-width: 85%; padding: 10px 14px; border-radius: 12px; line-height: 1.5; word-wrap: break-word; }
.message.user { align-self: flex-end; background: var(--primary); color: #fff; }
.message.assistant { align-self: flex-start; background: var(--msg-bg); color: var(--text); }
.message.assistant p { margin: 0 0 8px; }
.message.assistant p:last-child { margin-bottom: 0; }
.message.assistant code { background: var(--msg-code); padding: 2px 6px; border-radius: 4px; font-size: 0.9em; }
.message.assistant pre { background: var(--msg-code); padding: 8px 12px; border-radius: 6px; overflow-x: auto; }
.message.assistant pre code { background: none; padding: 0; }
.chat-input-area { border-top: 1px solid var(--border); padding: 12px; display: flex; gap: 8px; background: var(--surface); }
.chat-input-area input { flex: 1; border: 1px solid var(--input-border); border-radius: 8px; padding: 10px 14px; font-size: 14px; outline: none; background: var(--surface); color: var(--text); }
.chat-input-area input:focus { border-color: var(--primary); }
.chat-input-area button { background: var(--primary); color: #fff; border: none; border-radius: 8px; padding: 10px 20px; font-weight: 600; cursor: pointer; }
.chat-input-area button:disabled { opacity: .5; cursor: not-allowed; }
.typing { color: var(--text-muted); font-style: italic; font-size: 13px; padding: 0 16px 8px; }
@media (max-width: 600px) {
  .chat-container { height: 100vh; border-radius: 0; max-width: 100%; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<div class="chat-container">
  <div class="chat-header">
    {{if .LogoURL}}<img src="{{.LogoURL}}" alt="logo">{{end}}
    <h2>Консультант Сколково</h2>
    <button class="chat-header-btn" id="themeBtn" onclick="toggleTheme()" title="Переключить тему: светлая / тёмная">🌙</button>
  </div>
  <div class="chat-messages" id="messages"></div>
  <div class="typing" id="typing" style="display:none">Печатает…</div>
  <div class="chat-input-area">
    <input id="input" type="text" placeholder="Введите сообщение…" autocomplete="off" title="Нажмите Enter для отправки">
    <button id="send" disabled title="Отправить сообщение (Enter)">Отправить</button>
  </div>
</div>
<script>
(function(){
  var sessionId = null;
  var messagesEl = document.getElementById('messages');
  var inputEl = document.getElementById('input');
  var sendBtn = document.getElementById('send');
  var typingEl = document.getElementById('typing');

  function addMessage(role, text) {
    var div = document.createElement('div');
    div.className = 'message ' + role;
    if (role === 'assistant') {
      div.innerHTML = marked.parse(text || '');
    } else {
      div.textContent = text;
    }
    messagesEl.appendChild(div);
    messagesEl.scrollTop = messagesEl.scrollHeight;
  }

  function createSession() {
    return fetch('/api/session', {method:'POST'}).then(function(r){return r.json()}).then(function(d){sessionId = d.id;});
  }

  function sendMessage() {
    var text = inputEl.value.trim();
    if (!text || !sessionId) return;
    inputEl.value = '';
    addMessage('user', text);
    sendBtn.disabled = true;
    typingEl.style.display = 'block';

    fetch('/api/chat', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({session_id: sessionId, message: text})
    }).then(function(r){return r.json()}).then(function(d){
      typingEl.style.display = 'none';
      addMessage('assistant', d.reply || '');
      sendBtn.disabled = false;
    }).catch(function(){
      typingEl.style.display = 'none';
      addMessage('assistant', 'Ошибка соединения.');
      sendBtn.disabled = false;
    });
  }

  inputEl.addEventListener('input', function(){ sendBtn.disabled = !inputEl.value.trim(); });
  inputEl.addEventListener('keydown', function(e){ if(e.key==='Enter' && !e.shiftKey){ e.preventDefault(); sendMessage(); }});
  sendBtn.addEventListener('click', sendMessage);

  createSession().then(function(){ addMessage('assistant', '{{.WelcomeMessage}}'); });
})();

function toggleTheme() {
  var r = document.documentElement;
  var cur = r.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  var next = cur === 'dark' ? 'light' : 'dark';
  r.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  var btn = document.getElementById('themeBtn');
  if (btn) btn.textContent = next === 'dark' ? '☀️' : '🌙';
}
document.addEventListener('DOMContentLoaded', function() {
  var btn = document.getElementById('themeBtn');
  if (!btn) return;
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  btn.textContent = cur === 'dark' ? '☀️' : '🌙';
});
</script>
</body>
</html>`))

// serveChatPage отдаёт standalone-страницу чата.
func (s *WidgetServer) serveChatPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := chatPageHTML.Execute(w, s.config); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// widgetJS — inline JavaScript для встраивания в HTML.
const widgetJS = `
(function() {
  'use strict';

  var WIDGET_CSS =
    '#sk-widget-toggle{position:fixed;bottom:24px;right:24px;width:56px;height:56px;border-radius:50%;background:var(--sk-primary,#6366f1);border:none;color:#fff;cursor:pointer;box-shadow:0 4px 12px rgba(0,0,0,.2);z-index:99999;display:flex;align-items:center;justify-content:center;font-size:24px}'+
    '#sk-widget-panel{position:fixed;bottom:96px;right:24px;width:380px;max-width:calc(100vw - 48px);height:520px;max-height:calc(100vh - 120px);background:#fff;border-radius:16px;box-shadow:0 8px 32px rgba(0,0,0,.18);z-index:99999;display:none;flex-direction:column;overflow:hidden}'+
    '#sk-widget-panel.open{display:flex}'+
    '.sk-header{background:var(--sk-primary,#6366f1);color:#fff;padding:14px 16px;display:flex;align-items:center;gap:10px}'+
    '.sk-header img{height:28px;border-radius:4px}'+
    '.sk-header span{font-weight:600;font-size:15px}'+
    '.sk-messages{flex:1;overflow-y:auto;padding:12px;display:flex;flex-direction:column;gap:10px}'+
    '.sk-msg{max-width:85%;padding:8px 12px;border-radius:10px;line-height:1.45;font-size:13px;word-wrap:break-word}'+
    '.sk-msg.user{align-self:flex-end;background:var(--sk-primary,#6366f1);color:#fff}'+
    '.sk-msg.assistant{align-self:flex-start;background:#f3f4f6;color:#1f2937}'+
    '.sk-msg.assistant p{margin:0 0 6px}'+
    '.sk-msg.assistant p:last-child{margin-bottom:0}'+
    '.sk-msg.assistant code{background:#e5e7eb;padding:2px 5px;border-radius:3px;font-size:.9em}'+
    '.sk-msg.assistant pre{background:#e5e7eb;padding:6px 10px;border-radius:5px;overflow-x:auto}'+
    '.sk-msg.assistant pre code{background:none;padding:0}'+
    '.sk-input-row{border-top:1px solid #e5e7eb;padding:10px;display:flex;gap:6px}'+
    '.sk-input-row input{flex:1;border:1px solid #d1d5db;border-radius:8px;padding:8px 12px;font-size:13px;outline:none}'+
    '.sk-input-row button{background:var(--sk-primary,#6366f1);color:#fff;border:none;border-radius:8px;padding:8px 14px;font-weight:600;cursor:pointer;font-size:13px}'+
    '.sk-typing{color:#9ca3af;font-style:italic;font-size:12px;padding:0 12px 6px;display:none}';

  var WIDGET_HTML =
    '<button id="sk-widget-toggle" title="Открыть чат">&#128172;</button>'+
    '<div id="sk-widget-panel">'+
      '<div class="sk-header"><span class="sk-logo-wrap"></span><span>Консультант</span></div>'+
      '<div class="sk-messages" id="sk-msgs"></div>'+
      '<div class="sk-typing" id="sk-typing">Печатает&#8230;</div>'+
      '<div class="sk-input-row"><input id="sk-input" type="text" placeholder="Сообщение&#8230;" autocomplete="off"><button id="sk-send" title="Отправить сообщение (Enter)">Отправить</button></div>'+
    '</div>';

  function initWidget(cfg) {
    var base = cfg.base || '';
    var primary = cfg.primaryColor || '#6366f1';
    var welcome = cfg.welcomeMessage || 'Здравствуйте! Чем могу помочь?';
    var logoURL = cfg.logoURL || '';
    var sessionId = null;

    // Inject CSS
    var style = document.createElement('style');
    style.textContent = WIDGET_CSS.replace(/var\(--sk-primary,([^)]*)\)/g, primary);
    document.head.appendChild(style);

    document.documentElement.style.setProperty('--sk-primary', primary);

    // Inject HTML
    var wrapper = document.createElement('div');
    wrapper.innerHTML = WIDGET_HTML;
    document.body.appendChild(wrapper);

    var toggleBtn = document.getElementById('sk-widget-toggle');
    var panel = document.getElementById('sk-widget-panel');
    var msgsEl = document.getElementById('sk-msgs');
    var inputEl = document.getElementById('sk-input');
    var sendBtn = document.getElementById('sk-send');
    var typingEl = document.getElementById('sk-typing');

    if (logoURL) {
      var logoWrap = panel.querySelector('.sk-logo-wrap');
      var img = document.createElement('img');
      img.src = logoURL;
      logoWrap.appendChild(img);
    }

    function addMsg(role, text) {
      var div = document.createElement('div');
      div.className = 'sk-msg ' + role;
      if (role === 'assistant' && typeof marked !== 'undefined') {
        div.innerHTML = marked.parse(text || '');
      } else {
        div.textContent = text;
      }
      msgsEl.appendChild(div);
      msgsEl.scrollTop = msgsEl.scrollHeight;
    }

    function ensureSession() {
      if (sessionId) return Promise.resolve(sessionId);
      return fetch(base + '/api/session', {method:'POST'}).then(function(r){return r.json()}).then(function(d){sessionId=d.id; return sessionId;});
    }

    function doSend() {
      var text = inputEl.value.trim();
      if (!text) return;
      inputEl.value = '';
      sendBtn.disabled = true;
      addMsg('user', text);
      typingEl.style.display = 'block';

      ensureSession().then(function(sid) {
        return fetch(base + '/api/chat', {
          method:'POST',
          headers:{'Content-Type':'application/json'},
          body: JSON.stringify({session_id: sid, message: text})
        });
      }).then(function(r){return r.json()}).then(function(d){
        typingEl.style.display = 'none';
        addMsg('assistant', d.reply || '');
        sendBtn.disabled = false;
      }).catch(function(){
        typingEl.style.display = 'none';
        addMsg('assistant', 'Ошибка соединения.');
        sendBtn.disabled = false;
      });
    }

    toggleBtn.addEventListener('click', function(){ panel.classList.toggle('open'); });
    inputEl.addEventListener('input', function(){ sendBtn.disabled = !inputEl.value.trim(); });
    inputEl.addEventListener('keydown', function(e){ if(e.key==='Enter'&&!e.shiftKey){ e.preventDefault(); doSend(); }});
    sendBtn.addEventListener('click', doSend);

    ensureSession().then(function(){ addMsg('assistant', welcome); });
  }

  window.SkolkovoChat = { init: initWidget };

  // Auto-init from data attribute
  var scripts = document.querySelectorAll('script[data-skolkovo-chat]');
  if (scripts.length) {
    try {
      var cfg = JSON.parse(scripts[0].getAttribute('data-skolkovo-chat'));
      initWidget(cfg);
    } catch(e) { console.warn('skolkovo-chat parse error', e); }
  }
})();
`

// serveWidgetJS отдаёт JavaScript-файл виджета.
func (s *WidgetServer) serveWidgetJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	fmt.Fprint(w, widgetJS)
}

// chatRequest — тело запроса POST /api/chat.
type chatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

// chatResponse — тело ответа POST /api/chat.
type chatResponse struct {
	Reply string `json:"reply"`
}

// handleChat проксирует сообщение к MCP-серверу (tool: ask_consultant).
func (s *WidgetServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Формируем MCP-запрос: вызов tools/call с ask_consultant.
	mcpReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "ask_consultant",
			"arguments": map[string]any{
				"question":   req.Message,
				"session_id": req.SessionID,
			},
		},
	}

	mcpBody, err := json.Marshal(mcpReq)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	mcpURL := s.config.MCPURL + "/mcp"
	mcpReq_http, err := http.NewRequestWithContext(r.Context(), http.MethodPost, mcpURL, bytes.NewReader(mcpBody))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	mcpReq_http.Header.Set("Content-Type", "application/json")
	// Streamable HTTP MCP-сервер требует оба типа в Accept.
	mcpReq_http.Header.Set("Accept", "application/json, text/event-stream")
	if s.config.MCPAPIKey != "" {
		mcpReq_http.Header.Set("Authorization", "Bearer "+s.config.MCPAPIKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(mcpReq_http)
	if err != nil {
		http.Error(w, "mcp error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "mcp read error", http.StatusBadGateway)
		return
	}
	// Streamable HTTP может вернуть SSE — извлекаем JSON из строк data:.
	respBody = extractMCPJSON(respBody)

	// Парсим MCP-ответ.
	var mcpResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		http.Error(w, "mcp parse error", http.StatusBadGateway)
		return
	}
	if mcpResp.Error != nil {
		http.Error(w, "mcp: "+mcpResp.Error.Message, http.StatusBadGateway)
		return
	}

	var reply string
	for _, c := range mcpResp.Result.Content {
		if c.Type == "text" {
			reply += c.Text
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResponse{Reply: reply})
}

// extractMCPJSON возвращает JSON-полезную нагрузку из ответа MCP. Если тело —
// SSE-поток (text/event-stream), берёт содержимое последней строки «data:».
// Обычный JSON-ответ возвращается без изменений.
func extractMCPJSON(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed
	}
	var last []byte
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if data, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			last = bytes.TrimSpace(data)
		}
	}
	if len(last) > 0 {
		return last
	}
	return trimmed
}

// sessionResponse — тело ответа POST /api/session.
type sessionResponse struct {
	ID string `json:"id"`
}

// handleSession создаёт или возвращает сессию.
func (s *WidgetServer) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sess := s.sessionStore.GetOrCreate("")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionResponse{ID: sess.ID})
}
