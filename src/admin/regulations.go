// regulations.go — админ-панель для льгот (preferences) и НПА.
package admin

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"baza-skolkovo/src/common/model"
)

// prefView — строка таблицы для шаблона льгот.
type prefView struct {
	model.Preference
	StatusBadge string
}

// npaView — строка таблицы для шаблона НПА.
type npaView struct {
	model.NPADocument
	StatusBadge string
}

type regPageData struct {
	Preferences []prefView
	NPAs        []npaView
	PrefTypes   []prefTypeOption
	NPATypes    []npaTypeOption
	Flash       string
	FlashKind   string
}

type prefTypeOption struct {
	Value string
	Label string
}

type npaTypeOption struct {
	Value string
	Label string
}

var prefTypeOptions = []prefTypeOption{
	{"tax_profit", "Налог на прибыль"},
	{"insurance", "Страховые взносы"},
	{"vat", "НДС"},
	{"customs", "Таможня"},
	{"other", "Прочее"},
}

var npaTypeOptions = []npaTypeOption{
	{"law", "Федеральный закон"},
	{"decree", "Постановление"},
	{"order", "Приказ"},
	{"decision", "Решение"},
}

func (s *Server) handleRegulationsPage(w http.ResponseWriter, r *http.Request) {
	data := regPageData{
		PrefTypes: prefTypeOptions,
		NPATypes:  npaTypeOptions,
		Flash:     r.URL.Query().Get("msg"),
		FlashKind: r.URL.Query().Get("kind"),
	}

	if s.prefStore != nil {
		prefs, _ := s.prefStore.ListPreferences(r.Context(), "", "")
		for _, p := range prefs {
			pv := prefView{Preference: *p}
			switch p.Status {
			case "active":
				pv.StatusBadge = "badge-green"
			case "outdated":
				pv.StatusBadge = "badge-gray"
			default:
				pv.StatusBadge = "badge-gray"
			}
			data.Preferences = append(data.Preferences, pv)
		}
	}

	if s.npaStore != nil {
		npas, _ := s.npaStore.ListNPA(r.Context(), "", "")
		for _, n := range npas {
			nv := npaView{NPADocument: *n}
			switch n.Status {
			case "active":
				nv.StatusBadge = "badge-green"
			case "amended":
				nv.StatusBadge = "badge-yellow"
			case "revoked":
				nv.StatusBadge = "badge-red"
			default:
				nv.StatusBadge = "badge-gray"
			}
			data.NPAs = append(data.NPAs, nv)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := regulationsTmpl.Execute(w, data); err != nil {
		log.Println("[admin] шаблон regulations:", err)
	}
}

func (s *Server) handleCreatePreference(w http.ResponseWriter, r *http.Request) {
	if s.prefStore == nil {
		http.Redirect(w, r, "/regulations?msg=Хранилище+льгот+не+подключено&kind=err", http.StatusSeeOther)
		return
	}

	pref := &model.Preference{
		Title:       r.FormValue("title"),
		PrefType:    model.PreferenceType(r.FormValue("pref_type")),
		BenefitDesc: r.FormValue("benefit_desc"),
		LegalRef:    r.FormValue("legal_ref"),
		SourceURL:   r.FormValue("source_url"),
		Content:     r.FormValue("content"),
		Status:      model.PreferenceStatus(r.FormValue("status")),
		FetchedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if pref.Status == "" {
		pref.Status = "active"
	}
	if pref.PrefType == "" {
		pref.PrefType = "other"
	}

	if err := s.prefStore.CreatePreference(r.Context(), pref); err != nil {
		http.Redirect(w, r, "/regulations?msg="+err.Error()+"&kind=err", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/regulations?msg=Льгота+создана&kind=ok", http.StatusSeeOther)
}

func (s *Server) handleDeletePreference(w http.ResponseWriter, r *http.Request) {
	if s.prefStore == nil {
		http.Redirect(w, r, "/regulations?msg=Хранилище+льгот+не+подключено&kind=err", http.StatusSeeOther)
		return
	}
	id := r.PathValue("id")
	if err := s.prefStore.DeletePreference(r.Context(), id); err != nil {
		http.Redirect(w, r, "/regulations?msg="+err.Error()+"&kind=err", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/regulations?msg=Льгота+удалена&kind=ok", http.StatusSeeOther)
}

func (s *Server) handleCreateNPA(w http.ResponseWriter, r *http.Request) {
	if s.npaStore == nil {
		http.Redirect(w, r, "/regulations?msg=Хранилище+НПА+не+подключено&kind=err", http.StatusSeeOther)
		return
	}

	var issuedAt, effectiveAt time.Time
	if v := r.FormValue("issued_at"); v != "" {
		issuedAt, _ = time.Parse("2006-01-02", v)
	}
	if v := r.FormValue("effective_at"); v != "" {
		effectiveAt, _ = time.Parse("2006-01-02", v)
	}

	npa := &model.NPADocument{
		Title:       r.FormValue("title"),
		NPANumber:   r.FormValue("npa_number"),
		NPAType:     model.NPAType(r.FormValue("npa_type")),
		IssuedBy:    r.FormValue("issued_by"),
		IssuedAt:    issuedAt,
		EffectiveAt: effectiveAt,
		SourceURL:   r.FormValue("source_url"),
		Summary:     r.FormValue("summary"),
		Status:      model.NPAStatus(r.FormValue("status")),
		FetchedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if npa.Status == "" {
		npa.Status = "active"
	}

	if err := s.npaStore.CreateNPA(r.Context(), npa); err != nil {
		http.Redirect(w, r, "/regulations?msg="+err.Error()+"&kind=err", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/regulations?msg=НПА+создан&kind=ok", http.StatusSeeOther)
}

func (s *Server) handleDeleteNPA(w http.ResponseWriter, r *http.Request) {
	if s.npaStore == nil {
		http.Redirect(w, r, "/regulations?msg=Хранилище+НПА+не+подключено&kind=err", http.StatusSeeOther)
		return
	}
	id := r.PathValue("id")
	if err := s.npaStore.DeleteNPA(r.Context(), id); err != nil {
		http.Redirect(w, r, "/regulations?msg="+err.Error()+"&kind=err", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/regulations?msg=НПА+удален&kind=ok", http.StatusSeeOther)
}

// ===========================================================================
// Regulations HTML template
// ===========================================================================

var regulationsTmpl = template.Must(template.New("regulations").Funcs(template.FuncMap{
	"formatDate": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Format("02.01.2006")
	},
}).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>База Сколково — Льготы и НПА</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f5f6f8; --surface: #ffffff; --surface-alt: #f9fafb; --surface-hover: #f0f1f3;
  --primary: #0073ea; --primary-hover: #005bb5; --primary-light: #e8f2fc;
  --text: #1a1d29; --text-secondary: #676f83; --text-muted: #9498a8;
  --border: #e0e2e8; --border-strong: #c8cbd4;
  --radius: 8px; --shadow: 0 1px 3px rgba(0,0,0,.06);
  --green: #00875a; --green-bg: #e6f7f0; --green-border: #b3e0ce;
  --yellow: #bf6900; --yellow-bg: #fff6e5; --yellow-border: #f0d6a8;
  --red: #de350b; --red-bg: #fde8e0; --red-border: #f5b8a0;
  --gray: #676f83; --gray-bg: #f0f1f3; --gray-border: #d8dbe4;
}
body { font-family: 'Figtree', system-ui, sans-serif; background: var(--bg); color: var(--text); line-height: 1.5; padding: 20px; }
h1 { font-size: 24px; font-weight: 700; margin-bottom: 20px; }
h2 { font-size: 18px; font-weight: 600; margin: 24px 0 12px; }
.card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: var(--shadow); padding: 20px; margin-bottom: 20px; }
table { width: 100%; border-collapse: collapse; font-size: 13px; }
th { text-align: left; padding: 8px 12px; border-bottom: 2px solid var(--border-strong); font-weight: 600; color: var(--text-secondary); text-transform: uppercase; font-size: 11px; letter-spacing: .5px; }
td { padding: 8px 12px; border-bottom: 1px solid var(--border); vertical-align: top; }
tr:hover td { background: var(--surface-hover); }
.badge-green, .badge-yellow, .badge-red, .badge-gray {
  display: inline-block; padding: 2px 8px; border-radius: 10px; font-size: 11px; font-weight: 600;
}
.badge-green { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.badge-yellow { background: var(--yellow-bg); color: var(--yellow); border: 1px solid var(--yellow-border); }
.badge-red { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
.badge-gray { background: var(--gray-bg); color: var(--gray); border: 1px solid var(--gray-border); }
.btn { display: inline-flex; align-items: center; gap: 6px; padding: 6px 12px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); color: var(--text); font-size: 13px; cursor: pointer; text-decoration: none; }
.btn:hover { background: var(--surface-hover); border-color: var(--border-strong); }
.btn-primary { background: var(--primary); color: #fff; border-color: var(--primary); }
.btn-primary:hover { background: var(--primary-hover); }
.btn-sm { padding: 4px 8px; font-size: 12px; }
.btn-danger { color: var(--red); border-color: var(--red-border); }
.btn-danger:hover { background: var(--red-bg); }
form label { display: block; font-size: 12px; font-weight: 600; color: var(--text-secondary); margin-bottom: 4px; }
form input[type="text"], form input[type="date"], form input[type="url"], form textarea, form select {
  width: 100%; padding: 6px 10px; border: 1px solid var(--border); border-radius: var(--radius); font-size: 13px; font-family: inherit; background: var(--surface);
}
form textarea { min-height: 60px; resize: vertical; }
form .form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-bottom: 12px; }
form .form-group { margin-bottom: 12px; }
.flash { padding: 10px 16px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; }
.flash-ok { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.flash-err { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
a.source-link { color: var(--primary); text-decoration: none; font-size: 12px; }
a.source-link:hover { text-decoration: underline; }
.empty { text-align: center; padding: 40px 20px; color: var(--text-muted); }
.tabs { display: flex; gap: 4px; margin-bottom: 20px; border-bottom: 2px solid var(--border); }
.tab { padding: 10px 20px; cursor: pointer; font-weight: 600; color: var(--text-secondary); border-bottom: 2px solid transparent; margin-bottom: -2px; }
.tab.active { color: var(--primary); border-bottom-color: var(--primary); }
.tab:hover { color: var(--primary); }
.tab-content { display: none; }
.tab-content.active { display: block; }
</style>
</head>
<body>
<a href="/" style="color:var(--primary);text-decoration:none;font-size:14px">← Назад в админку</a>
<h1>📜 Льготы и НПА</h1>

{{if .Flash}}<div class="flash flash-{{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="tabs">
  <div class="tab active" onclick="showTab('prefs',this)">💼 Льготы ({{len .Preferences}})</div>
  <div class="tab" onclick="showTab('npas',this)">📋 НПА ({{len .NPAs}})</div>
</div>

<div id="prefs" class="tab-content active">
  <div class="card">
    <h2>Добавить льготу</h2>
    <form method="POST" action="/regulations/preferences">
      <div class="form-row">
        <div class="form-group">
          <label>Название *</label>
          <input type="text" name="title" required placeholder="Например: Льгота по налогу на прибыль">
        </div>
        <div class="form-group">
          <label>Тип льготы</label>
          <select name="pref_type">
            {{range .PrefTypes}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
          </select>
        </div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>Ссылка на источник *</label>
          <input type="url" name="source_url" required placeholder="https://...">
        </div>
        <div class="form-group">
          <label>Ссылка на НПА</label>
          <input type="text" name="legal_ref" placeholder="244-ФЗ, ст. 1">
        </div>
      </div>
      <div class="form-group">
        <label>Описание льготы</label>
        <textarea name="benefit_desc" placeholder="Краткое описание льготы..."></textarea>
      </div>
      <div class="form-group">
        <label>Полный текст</label>
        <textarea name="content" placeholder="Полный текст раздела о льготе..."></textarea>
      </div>
      <div class="form-group">
        <label>Статус</label>
        <select name="status">
          <option value="active">Действует</option>
          <option value="outdated">Устарела</option>
        </select>
      </div>
      <button type="submit" class="btn btn-primary">Создать льготу</button>
    </form>
  </div>

  <div class="card">
    <h2>Реестр льгот</h2>
    {{if .Preferences}}
    <div class="table-wrap">
      <table>
        <thead><tr><th>Название</th><th>Тип</th><th>Статус</th><th>Источник</th><th>Действия</th></tr></thead>
        <tbody>
        {{range .Preferences}}
          <tr>
            <td style="font-weight:500">{{.Title}}</td>
            <td style="font-size:12px">{{.PrefType}}</td>
            <td><span class="{{.StatusBadge}}">{{.Status}}</span></td>
            <td>{{if .SourceURL}}<a href="{{.SourceURL}}" target="_blank" class="source-link">источник ↗</a>{{else}}—{{end}}</td>
            <td>
              <form method="POST" action="/regulations/preferences/{{.ID}}/delete" style="display:inline" onsubmit="return confirm('Удалить льготу?')">
                <button type="submit" class="btn btn-sm btn-danger">Удалить</button>
              </form>
            </td>
          </tr>
        {{end}}
        </tbody>
      </table>
    </div>
    {{else}}
    <div class="empty">Льготы ещё не добавлены</div>
    {{end}}
  </div>
</div>

<div id="npas" class="tab-content">
  <div class="card">
    <h2>Добавить НПА</h2>
    <form method="POST" action="/regulations/npa">
      <div class="form-row">
        <div class="form-group">
          <label>Название *</label>
          <input type="text" name="title" required placeholder="Например: ФЗ О Сколково">
        </div>
        <div class="form-group">
          <label>Номер НПА</label>
          <input type="text" name="npa_number" placeholder="244-ФЗ">
        </div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>Тип НПА</label>
          <select name="npa_type">
            {{range .NPATypes}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
          </select>
        </div>
        <div class="form-group">
          <label>Орган-издатель</label>
          <input type="text" name="issued_by" placeholder="Государственная Дума">
        </div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>Ссылка на источник *</label>
          <input type="url" name="source_url" required placeholder="https://...">
        </div>
        <div class="form-group">
          <label>Статус</label>
          <select name="status">
            <option value="active">Действует</option>
            <option value="amended">С изменениями</option>
            <option value="revoked">Отменён</option>
          </select>
        </div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>Дата принятия</label>
          <input type="date" name="issued_at">
        </div>
        <div class="form-group">
          <label>Дата вступления в силу</label>
          <input type="date" name="effective_at">
        </div>
      </div>
      <div class="form-group">
        <label>Краткое содержание</label>
        <textarea name="summary" placeholder="Краткое содержание НПА..."></textarea>
      </div>
      <button type="submit" class="btn btn-primary">Создать НПА</button>
    </form>
  </div>

  <div class="card">
    <h2>Реестр НПА</h2>
    {{if .NPAs}}
    <div class="table-wrap">
      <table>
        <thead><tr><th>Название</th><th>Номер</th><th>Тип</th><th>Статус</th><th>Источник</th><th>Действия</th></tr></thead>
        <tbody>
        {{range .NPAs}}
          <tr>
            <td style="font-weight:500">{{.Title}}</td>
            <td style="font-size:12px;font-family:monospace">{{if .NPANumber}}{{.NPANumber}}{{else}}—{{end}}</td>
            <td style="font-size:12px">{{.NPAType}}</td>
            <td><span class="{{.StatusBadge}}">{{.Status}}</span></td>
            <td>{{if .SourceURL}}<a href="{{.SourceURL}}" target="_blank" class="source-link">источник ↗</a>{{else}}—{{end}}</td>
            <td>
              <form method="POST" action="/regulations/npa/{{.ID}}/delete" style="display:inline" onsubmit="return confirm('Удалить НПА?')">
                <button type="submit" class="btn btn-sm btn-danger">Удалить</button>
              </form>
            </td>
          </tr>
        {{end}}
        </tbody>
      </table>
    </div>
    {{else}}
    <div class="empty">НПА ещё не добавлены</div>
    {{end}}
  </div>
</div>

<script>
function showTab(id, el) {
  document.querySelectorAll('.tab-content').forEach(function(t) { t.classList.remove('active'); });
  document.querySelectorAll('.tab').forEach(function(t) { t.classList.remove('active'); });
  document.getElementById(id).classList.add('active');
  el.classList.add('active');
}
</script>
</body>
</html>
`))
