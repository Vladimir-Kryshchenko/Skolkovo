package admin

import "html/template"

var tmpl = template.Must(template.New("admin").Parse(`
{{define "layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<title>База Сколково — Админка</title>
<style>
  body { font-family: -apple-system, Segoe UI, Roboto, sans-serif; margin: 0; background:#f5f6f8; color:#1c2330; }
  header { background:#0b2e6e; color:#fff; padding:14px 24px; }
  header h1 { margin:0; font-size:18px; }
  main { padding:24px; }
  .bar { display:flex; gap:12px; align-items:center; margin-bottom:16px; flex-wrap:wrap; }
  .stats { display:flex; gap:10px; flex-wrap:wrap; margin-bottom:18px; }
  .stat { background:#fff; border-radius:8px; padding:10px 16px; min-width:90px; }
  .stat .n { font-size:20px; font-weight:600; }
  .stat .l { font-size:12px; color:#5f6368; }
  .flash { padding:10px 14px; border-radius:6px; margin-bottom:16px; }
  .flash.ok { background:#e6f4ea; color:#137333; }
  .flash.err { background:#fce8e6; color:#c5221f; }
  table { width:100%; border-collapse:collapse; background:#fff; border-radius:8px; overflow:hidden; }
  th, td { text-align:left; padding:10px 12px; border-bottom:1px solid #eceef1; font-size:13px; vertical-align:top; }
  th { background:#f0f2f5; }
  .badge { padding:2px 8px; border-radius:10px; font-size:11px; white-space:nowrap; }
  .s-на_проверке { background:#fef7e0; color:#b06000; }
  .s-действует { background:#e6f4ea; color:#137333; }
  .s-устарел { background:#fce8e6; color:#c5221f; }
  .s-архив { background:#eceff1; color:#5f6368; }
  .s-отклонён { background:#f3e8fd; color:#8430ce; }
  form.inline { display:inline; }
  select, input[type=text] { padding:4px 6px; border:1px solid #ccd0d5; border-radius:4px; font-size:12px; }
  button { padding:4px 10px; border:none; border-radius:4px; background:#0b57d0; color:#fff; cursor:pointer; font-size:12px; }
  button.danger { background:#c5221f; }
  a { color:#0b57d0; }
  .muted { color:#80868b; font-size:12px; }
  .row-actions form { margin-bottom:4px; }
</style>
</head>
<body>
<header><h1>📚 База Сколково — Админка документов</h1></header>
<main>{{template "content" .}}</main>
</body>
</html>{{end}}

{{define "content"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="stats">
  <div class="stat"><div class="n">{{.Stats.Total}}</div><div class="l">всего</div></div>
  <div class="stat"><div class="n">{{.Stats.Active}}</div><div class="l">действует</div></div>
  <div class="stat"><div class="n">{{.Stats.Pending}}</div><div class="l">на проверке</div></div>
  <div class="stat"><div class="n">{{.Stats.Outdated}}</div><div class="l">устарело</div></div>
  <div class="stat"><div class="n">{{.Stats.Indexed}}</div><div class="l">в индексе</div></div>
</div>

<div class="bar">
  <span class="muted">Статус:</span>
  <a href="/">все</a>
  <a href="/?status=на_проверке">на проверке</a>
  <a href="/?status=действует">действует</a>
  <a href="/?status=устарел">устарел</a>
  <a href="/?status=архив">архив</a>
  <form class="inline" method="get" action="/">
    <input type="text" name="q" value="{{.Query}}" placeholder="поиск по названию">
    <button type="submit">Найти</button>
  </form>
</div>

<table>
  <tr><th>Документ</th><th>Категория</th><th>Статус</th><th>Файл</th><th>Индекс</th><th>Действия</th></tr>
  {{range .Docs}}
  <tr>
    <td>
      <div><strong>{{.Title}}</strong></div>
      <div class="muted"><a href="{{.SourceURL}}" target="_blank">источник</a> · id {{.ID}}{{if .Supersedes}} · заменяет {{.Supersedes}}{{end}}</div>
    </td>
    <td>
      <form class="inline" method="post" action="/documents/{{.ID}}/category">
        <input type="text" name="category" value="{{.Category}}" placeholder="категория">
        <button type="submit">✓</button>
      </form>
    </td>
    <td><span class="badge s-{{.Status}}">{{.Status}}</span></td>
    <td>
      {{if .LocalPath}}📄{{else}}
      <form class="inline" method="post" action="/documents/{{.ID}}/upload" enctype="multipart/form-data">
        <input type="file" name="file" required style="width:130px">
        <button type="submit">⬆</button>
      </form>
      {{end}}
    </td>
    <td>{{if .Indexed}}✅{{else}}—{{end}}</td>
    <td class="row-actions">
      <form class="inline" method="post" action="/documents/{{.ID}}/status">
        <select name="status">
          <option value="на_проверке" {{if eq .StatusStr "на_проверке"}}selected{{end}}>на проверке</option>
          <option value="действует" {{if eq .StatusStr "действует"}}selected{{end}}>действует</option>
          <option value="устарел" {{if eq .StatusStr "устарел"}}selected{{end}}>устарел</option>
          <option value="архив" {{if eq .StatusStr "архив"}}selected{{end}}>архив</option>
          <option value="отклонён" {{if eq .StatusStr "отклонён"}}selected{{end}}>отклонён</option>
        </select>
        <button type="submit">Применить</button>
      </form>
      <form class="inline" method="post" action="/documents/{{.ID}}/supersedes">
        <input type="text" name="supersedes" value="{{.Supersedes}}" placeholder="id заменяемого">
        <button type="submit">Заменяет</button>
      </form>
      <form class="inline" method="post" action="/documents/{{.ID}}/delete" onsubmit="return confirm('Удалить документ?')">
        <button class="danger" type="submit">Удалить</button>
      </form>
    </td>
  </tr>
  {{else}}
  <tr><td colspan="6" class="muted">Нет документов. Запустите парсинг: <code>skolkovo scrape</code></td></tr>
  {{end}}
</table>
{{end}}
`))
