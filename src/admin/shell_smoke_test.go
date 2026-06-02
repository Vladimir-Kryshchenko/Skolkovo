package admin

import (
	"io"
	"strings"
	"testing"
)

// isEscapeErr распознаёт именно ошибку контекстного экранирования html/template
// (а не обычную ошибку доступа к данным). Экранирование выполняется один раз при
// первом Execute и не зависит от данных, поэтому если шаблон вернул ошибку про
// поле/данные — значит экранирование уже успешно прошло.
func isEscapeErr(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "escape") ||
		strings.Contains(m, "ambiguous context") ||
		strings.Contains(m, "ends in a non-text context") ||
		strings.Contains(m, "branches end in different")
}

// execNoEscapeErr выполняет шаблон с пустыми данными: паника/ошибка по данным
// допустимы (экранирование к этому моменту уже прошло), а вот ошибка экранирования —
// это баг каркаса (например, сломанный {{template "sidebar" .}}).
func execNoEscapeErr(t *testing.T, name string, fn func() error) {
	t.Helper()
	defer func() { _ = recover() }()
	if err := fn(); err != nil && isEscapeErr(err) {
		t.Errorf("%s: ошибка экранирования html/template (сломан каркас сайдбара): %v", name, err)
	}
}

// TestSidebarTemplatesEscapeClean проверяет, что внедрённый левый сайдбар не ломает
// контекстное экранирование на страницах, не покрытых остальными шаблонными тестами:
// админка резидентства (:8091), ИИ-конфигурация и регламенты (:8090).
func TestSidebarTemplatesEscapeClean(t *testing.T) {
	// Резидентство (:8091)
	execNoEscapeErr(t, "residencyTmpl", func() error { return residencyTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "clientCardTmpl", func() error { return clientCardTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "checklistsTmpl", func() error { return checklistsTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "deadlinesTmpl", func() error { return deadlinesTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "templatesTmpl", func() error { return templatesTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "tenantsTmpl", func() error { return tenantsTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "eventsTmpl", func() error { return eventsTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "contestsTmpl", func() error { return contestsTmpl.Execute(io.Discard, nil) })

	// ИИ-конфигурация (:8090)
	execNoEscapeErr(t, "aiTmpl/ai-layout", func() error { return aiTmpl.ExecuteTemplate(io.Discard, "ai-layout", nil) })
	execNoEscapeErr(t, "aiModelsTmpl", func() error { return aiModelsTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "aiModelFormTmpl", func() error { return aiModelFormTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "aiAgentsTmpl", func() error { return aiAgentsTmpl.Execute(io.Discard, nil) })
	execNoEscapeErr(t, "aiAgentFormTmpl", func() error { return aiAgentFormTmpl.Execute(io.Discard, nil) })

	// Регламенты/НПА (:8090)
	execNoEscapeErr(t, "regulationsTmpl", func() error { return regulationsTmpl.Execute(io.Discard, nil) })

	// Страница прокси собирается строкой (не html/template) — убеждаемся, что сайдбар вшит
	// и сборка не паникует.
	out := renderProxyPage(map[string]interface{}{"Proxies": []ProxyConfig{}, "ActiveID": ""})
	if !strings.Contains(out, "app-sidebar") {
		t.Error("renderProxyPage: левый сайдбар не вставлен в страницу прокси")
	}
}
