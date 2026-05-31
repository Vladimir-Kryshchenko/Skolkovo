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
	// PrimaryColor — основной цвет виджета (CSS-цвет), по умолчанию "#0073ea".
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
		config.PrimaryColor = "#0073ea"
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
<style>
:root {
  --primary: {{.PrimaryColor}};
  --primary-hover: #005fbf;
  --bg: #f0f2f5;
  --surface: #ffffff;
  --msg-user: {{.PrimaryColor}};
  --msg-assistant: #f0f2f5;
  --msg-code: #e8eaed;
  --text: #181b2b;
  --text-on-primary: #ffffff;
  --text-muted: #6b7280;
  --border: #e0e3eb;
  --input-border: #d1d5db;
  --shadow: 0 2px 8px rgba(0,0,0,0.08);
  --shadow-lg: 0 4px 24px rgba(0,0,0,0.12);
  --radius: 8px;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b;
    --surface: #23273a;
    --msg-user: {{.PrimaryColor}};
    --msg-assistant: #2c3044;
    --msg-code: #363b50;
    --text: #e8eaed;
    --text-on-primary: #ffffff;
    --text-muted: #9ca3af;
    --border: #333850;
    --input-border: #3e4460;
    --shadow: 0 2px 8px rgba(0,0,0,0.3);
    --shadow-lg: 0 4px 24px rgba(0,0,0,0.4);
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b;
  --surface: #23273a;
  --msg-user: {{.PrimaryColor}};
  --msg-assistant: #2c3044;
  --msg-code: #363b50;
  --text: #e8eaed;
  --text-on-primary: #ffffff;
  --text-muted: #9ca3af;
  --border: #333850;
  --input-border: #3e4460;
  --shadow: 0 2px 8px rgba(0,0,0,0.3);
  --shadow-lg: 0 4px 24px rgba(0,0,0,0.4);
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: var(--bg);
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  padding: 16px;
  color: var(--text);
}

/* Tooltip */
[data-tooltip] { position: relative; }
[data-tooltip]::after {
  content: attr(data-tooltip);
  position: absolute;
  bottom: calc(100% + 6px);
  left: 50%;
  transform: translateX(-50%);
  background: var(--text);
  color: var(--bg);
  font-size: 11px;
  font-weight: 500;
  padding: 4px 8px;
  border-radius: 4px;
  white-space: nowrap;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.15s ease;
  z-index: 10;
}
[data-tooltip]:hover::after { opacity: 1; }

.chat-container {
  width: 100%;
  max-width: 520px;
  background: var(--surface);
  border-radius: var(--radius);
  box-shadow: var(--shadow-lg);
  display: flex;
  flex-direction: column;
  height: 80vh;
  max-height: 700px;
  overflow: hidden;
}

/* Header */
.chat-header {
  background: var(--primary);
  color: var(--text-on-primary);
  padding: 14px 18px;
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}
.chat-header img { height: 28px; border-radius: 4px; }
.chat-header h2 { font-size: 16px; font-weight: 600; flex: 1; }
.chat-header-btn {
  background: rgba(255,255,255,0.15);
  color: var(--text-on-primary);
  border: 1px solid rgba(255,255,255,0.25);
  border-radius: 6px;
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background 0.15s;
}
.chat-header-btn:hover { background: rgba(255,255,255,0.25); }
.chat-header-btn svg { width: 18px; height: 18px; fill: currentColor; }

/* Messages */
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  scrollbar-width: thin;
  scrollbar-color: var(--border) transparent;
}
.message {
  max-width: 85%;
  padding: 10px 14px;
  border-radius: var(--radius);
  line-height: 1.5;
  word-break: break-word;
  overflow-wrap: break-word;
}
.message.user {
  align-self: flex-end;
  background: var(--msg-user);
  color: var(--text-on-primary);
}
.message.assistant {
  align-self: flex-start;
  background: var(--msg-assistant);
  color: var(--text);
}
.message.assistant p { margin: 0 0 8px; }
.message.assistant p:last-child { margin-bottom: 0; }
.message.assistant code {
  background: var(--msg-code);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 0.9em;
  overflow-wrap: anywhere;
}
.message.assistant pre {
  background: var(--msg-code);
  padding: 8px 12px;
  border-radius: 6px;
  overflow-x: auto;
}
.message.assistant pre code { background: none; padding: 0; }

/* Typing indicator */
.typing {
  color: var(--text-muted);
  font-style: italic;
  font-size: 13px;
  padding: 0 16px 8px;
}

/* Input area */
.chat-input-area {
  border-top: 1px solid var(--border);
  padding: 12px;
  display: flex;
  gap: 8px;
  background: var(--surface);
  flex-shrink: 0;
}
.chat-input-area input {
  flex: 1;
  border: 1px solid var(--input-border);
  border-radius: var(--radius);
  padding: 10px 14px;
  font-size: 14px;
  font-family: inherit;
  outline: none;
  background: var(--surface);
  color: var(--text);
  transition: border-color 0.15s;
}
.chat-input-area input:focus { border-color: var(--primary); }
.chat-input-area button {
  background: var(--primary);
  color: var(--text-on-primary);
  border: none;
  border-radius: var(--radius);
  padding: 10px 20px;
  font-weight: 600;
  font-family: inherit;
  font-size: 14px;
  cursor: pointer;
  transition: background 0.15s;
}
.chat-input-area button:hover { background: var(--primary-hover); }
.chat-input-area button:disabled { opacity: 0.5; cursor: not-allowed; }

/* Responsive */
@media (max-width: 768px) {
  body { padding: 0; }
  .chat-container {
    height: 100vh;
    max-height: none;
    border-radius: 0;
    max-width: 100%;
  }
}
@media (max-width: 480px) {
  .chat-header { padding: 12px 14px; gap: 8px; }
  .chat-header h2 { font-size: 14px; }
  .chat-messages { padding: 12px; }
  .chat-input-area { padding: 10px; }
  .chat-input-area button { padding: 10px 14px; font-size: 13px; }
}
@media (min-width: 1024px) {
  .chat-container { max-width: 560px; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<div class="chat-container">
  <div class="chat-header">
    {{if .LogoURL}}<img src="{{.LogoURL}}" alt="Логотип">{{end}}
    <h2>Консультант Сколково</h2>
    <button class="chat-header-btn" id="themeBtn" onclick="toggleTheme()" data-tooltip="Переключить тему">
      <svg id="themeIcon" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"></svg>
    </button>
  </div>
  <div class="chat-messages" id="messages"></div>
  <div class="typing" id="typing" style="display:none">Печатает&#8230;</div>
  <div class="chat-input-area">
    <input id="input" type="text" placeholder="Введите сообщение&#8230;" autocomplete="off" data-tooltip="Нажмите Enter для отправки">
    <button id="send" disabled data-tooltip="Отправить сообщение">Отправить</button>
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

// SVG icons for theme
var moonSVG = '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>';
var sunSVG = '<circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>';

function setThemeIcon(isDark) {
  document.getElementById('themeIcon').innerHTML = isDark ? sunSVG : moonSVG;
}

function toggleTheme() {
  var r = document.documentElement;
  var cur = r.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  var next = cur === 'dark' ? 'light' : 'dark';
  r.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  setThemeIcon(next === 'dark');
}

function applyThemeIcon() {
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  setThemeIcon(cur === 'dark');
}

document.addEventListener('DOMContentLoaded', applyThemeIcon);
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
    '#sk-widget-toggle{position:fixed;bottom:24px;right:24px;width:52px;height:52px;border-radius:8px;background:var(--sk-primary,#0073ea);border:none;color:#fff;cursor:pointer;box-shadow:0 2px 8px rgba(0,0,0,.18);z-index:99999;display:flex;align-items:center;justify-content:center;transition:background .15s}'+
    '#sk-widget-toggle:hover{background:var(--sk-hover,#005fbf)}'+
    '#sk-widget-toggle svg{width:22px;height:22px;fill:currentColor}'+
    '#sk-widget-panel{position:fixed;bottom:88px;right:24px;width:380px;max-width:calc(100vw - 48px);height:520px;max-height:calc(100vh - 120px);background:var(--sk-surface,#23273a);border-radius:8px;box-shadow:0 4px 24px rgba(0,0,0,.3);z-index:99999;display:none;flex-direction:column;overflow:hidden;font-family:Figtree,-apple-system,BlinkMacSystemFont,sans-serif;border:1px solid var(--sk-border,#333850)}'+
    '#sk-widget-panel.open{display:flex}'+
    '.sk-header{background:var(--sk-primary,#0073ea);color:#fff;padding:12px 16px;display:flex;align-items:center;gap:10px;flex-shrink:0}'+
    '.sk-header img{height:26px;border-radius:4px}'+
    '.sk-header span{font-weight:600;font-size:14px}'+
    '.sk-header-close{margin-left:auto;background:rgba(255,255,255,.15);border:1px solid rgba(255,255,255,.25);border-radius:6px;width:30px;height:30px;display:flex;align-items:center;justify-content:center;cursor:pointer;color:#fff;transition:background .15s}'+
    '.sk-header-close:hover{background:rgba(255,255,255,.25)}'+
    '.sk-messages{flex:1;overflow-y:auto;padding:14px;display:flex;flex-direction:column;gap:10px;scrollbar-width:thin;scrollbar-color:#333850 transparent}'+
    '.sk-msg{max-width:85%;padding:9px 13px;border-radius:8px;line-height:1.5;font-size:13px;word-break:break-word;overflow-wrap:break-word}'+
    '.sk-msg.user{align-self:flex-end;background:var(--sk-primary,#0073ea);color:#fff}'+
    '.sk-msg.assistant{align-self:flex-start;background:var(--sk-msg-bg,#2c3044);color:var(--sk-text,#e8eaed)}'+
    '.sk-msg.assistant p{margin:0 0 6px}'+
    '.sk-msg.assistant p:last-child{margin-bottom:0}'+
    '.sk-msg.assistant code{background:var(--sk-code-bg,#363b50);padding:2px 5px;border-radius:3px;font-size:.9em;overflow-wrap:anywhere}'+
    '.sk-msg.assistant pre{background:var(--sk-code-bg,#363b50);padding:6px 10px;border-radius:5px;overflow-x:auto}'+
    '.sk-msg.assistant pre code{background:none;padding:0}'+
    '.sk-input-row{border-top:1px solid var(--sk-border,#333850);padding:10px;display:flex;gap:6px;background:var(--sk-surface,#23273a);flex-shrink:0}'+
    '.sk-input-row input{flex:1;border:1px solid var(--sk-input-border,#3e4460);border-radius:6px;padding:8px 12px;font-size:13px;outline:none;font-family:inherit;background:var(--sk-surface,#23273a);color:var(--sk-text,#e8eaed)}'+
    '.sk-input-row input:focus{border-color:var(--sk-primary,#0073ea)}'+
    '.sk-input-row button{background:var(--sk-primary,#0073ea);color:#fff;border:none;border-radius:6px;padding:8px 14px;font-weight:600;cursor:pointer;font-size:13px;font-family:inherit;transition:background .15s}'+
    '.sk-input-row button:hover{background:var(--sk-hover,#005fbf)}'+
    '.sk-typing{color:var(--sk-muted,#9ca3af);font-style:italic;font-size:12px;padding:0 14px 6px;display:none}'+
    '@media(max-width:480px){#sk-widget-panel{bottom:0;right:0;width:100%;height:100%;max-width:100%;max-height:100%;border-radius:0}}';

  var CHAT_SVG = '<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>';
  var CLOSE_SVG = '<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" width="16" height="16"><line x1="18" y1="6" x2="6" y2="18" stroke="currentColor" stroke-width="2"/><line x1="6" y1="6" x2="18" y2="18" stroke="currentColor" stroke-width="2"/></svg>';

  var WIDGET_HTML =
    '<button id="sk-widget-toggle" title="Открыть чат">' + CHAT_SVG + '</button>'+
    '<div id="sk-widget-panel">'+
      '<div class="sk-header"><span class="sk-logo-wrap"></span><span>Консультант</span><button class="sk-header-close" title="Закрыть чат">' + CLOSE_SVG + '</button></div>'+
      '<div class="sk-messages" id="sk-msgs"></div>'+
      '<div class="sk-typing" id="sk-typing">Печатает&#8230;</div>'+
      '<div class="sk-input-row"><input id="sk-input" type="text" placeholder="Сообщение&#8230;" autocomplete="off"><button id="sk-send" title="Отправить сообщение (Enter)">Отправить</button></div>'+
    '</div>';

  function initWidget(cfg) {
    var base = cfg.base || '';
    var primary = cfg.primaryColor || '#0073ea';
    var welcome = cfg.welcomeMessage || 'Здравствуйте! Чем могу помочь?';
    var logoURL = cfg.logoURL || '';
    var sessionId = null;

    // Inject CSS
    var style = document.createElement('style');
    style.textContent = WIDGET_CSS.replace(/var\(--sk-primary,([^)]*)\)/g, primary);
    document.head.appendChild(style);

    // Палитра виджета адаптируется к теме: cfg.theme ('light'|'dark') либо
    // системная prefers-color-scheme хост-страницы (по умолчанию).
    document.documentElement.style.setProperty('--sk-primary', primary);
    document.documentElement.style.setProperty('--sk-hover', '#005fbf');
    var SK_THEMES = {
      light: { surface:'#ffffff', text:'#323338', muted:'#676879', border:'#e0e2e8', inputBorder:'#c3c6d4', msgBg:'#f0f2f5', codeBg:'#eef0f4' },
      dark:  { surface:'#23273a', text:'#e8eaed', muted:'#9ca3af', border:'#333850', inputBorder:'#3e4460', msgBg:'#2c3044', codeBg:'#363b50' }
    };
    function applySkTheme(scheme) {
      var t = SK_THEMES[scheme] || SK_THEMES.light;
      var s = document.documentElement.style;
      s.setProperty('--sk-surface', t.surface);
      s.setProperty('--sk-text', t.text);
      s.setProperty('--sk-muted', t.muted);
      s.setProperty('--sk-border', t.border);
      s.setProperty('--sk-input-border', t.inputBorder);
      s.setProperty('--sk-msg-bg', t.msgBg);
      s.setProperty('--sk-code-bg', t.codeBg);
    }
    var mq = window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null;
    if (cfg.theme === 'light' || cfg.theme === 'dark') {
      applySkTheme(cfg.theme);
    } else {
      applySkTheme(mq && mq.matches ? 'dark' : 'light');
      if (mq && mq.addEventListener) {
        mq.addEventListener('change', function(e) { applySkTheme(e.matches ? 'dark' : 'light'); });
      }
    }

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
      img.alt = 'Логотип';
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
    panel.querySelector('.sk-header-close').addEventListener('click', function(){ panel.classList.remove('open'); });
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
