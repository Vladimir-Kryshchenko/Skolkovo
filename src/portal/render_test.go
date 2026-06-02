package portal

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
)

// testClient — представительный клиент-резидент для рендера страниц портала.
func testClient() *model.Client {
	return &model.Client{
		ID:             "client-1",
		Name:           "ООО Тест",
		INN:            "7700000000",
		ContactEmail:   "resident@example.ru",
		ResidencyStage: model.StageResident,
	}
}

// testDeadlines — пара дедлайнов: предстоящий, просроченный и выполненный.
func testDeadlines() []*model.Deadline {
	now := time.Now()
	return []*model.Deadline{
		{ID: "d1", Title: "Подать отчёт", DueDate: now.AddDate(0, 0, 14), Type: model.DeadlineReporting, Status: model.DeadlineUpcoming},
		{ID: "d2", Title: "Просроченный отчёт", DueDate: now.AddDate(0, 0, -3), Type: model.DeadlineReporting, Status: model.DeadlineUpcoming},
		{ID: "d3", Title: "Закрытый дедлайн", DueDate: now.AddDate(0, 0, -10), Type: model.DeadlineApplication, Status: model.DeadlineCompleted},
	}
}

// testChecklists — пара обогащённых чек-листов с шагами.
func testChecklists() []*checklistView {
	return []*checklistView{
		{
			ID:             "cc1",
			Title:          "Вступление в резиденты",
			ProcedureType:  "Вступление",
			Status:         model.ChecklistInProgress,
			Progress:       50,
			CompletedSteps: 1,
			TotalSteps:     2,
			Steps: []checklistStepView{
				{Title: "Подать заявку", Status: string(model.StepDone)},
				{Title: "Подписать договор", Status: string(model.StepInProgress)},
			},
		},
		{
			ID:            "cc2",
			Title:         "Отчётность",
			ProcedureType: "Отчётность",
			Status:        model.ChecklistNotStarted,
			Progress:      0,
			TotalSteps:    1,
			Steps: []checklistStepView{
				{Title: "Сдать годовой отчёт", Status: string(model.StepPending)},
			},
		},
	}
}

// testDocuments — пара документов сопровождения.
func testDocuments() []*portalDocInfo {
	return []*portalDocInfo{
		{ID: "pd1", Name: "Заявление", Role: "required", Status: "approved", StatusClass: "approved", StatusLabel: "Утверждён", Date: "01.06.2026", SourceURL: "https://sk.ru/doc1"},
		{ID: "pd2", Name: "Отчёт", Role: "submitted", Status: "pending", StatusClass: "pending", StatusLabel: "Ожидает"},
	}
}

func testTemplates() []*model.DocumentTemplate {
	return []*model.DocumentTemplate{
		{ID: "tpl1", Name: "Заявление на вступление", Type: "entry", Version: "1.0"},
		{ID: "tpl2", Name: "Годовой отчёт", Type: "reporting", Version: "2.0"},
	}
}

// base строит baseData для авторизованной страницы.
func base(page string) baseData {
	return baseData{Client: testClient(), Flash: "Тестовое сообщение", FlashKind: "ok", Page: page}
}

// pageCase описывает один сценарий рендера авторизованной страницы.
type pageCase struct {
	name string
	tmpl string
	data interface{}
}

// authPages — все авторизованные страницы с непустыми данными.
func authPages() []pageCase {
	return []pageCase{
		{"dashboard", "dashboard", dashboardData{
			baseData:            base("dashboard"),
			Deadlines:           testDeadlines(),
			Checklists:          testChecklists(),
			Documents:           testDocuments(),
			TelegramBotUsername: "@SkolkovoBot",
			RecentChanges:       []recentChange{{Title: "Новый документ", Kind: "new", EntityType: "document", DetectedAt: "сегодня", Summary: "Появился новый НПА"}},
		}},
		{"checklists", "checklists", checklistsData{
			baseData:   base("checklists"),
			Checklists: testChecklists(),
		}},
		{"deadlines", "deadlines", deadlinesData{
			baseData:  base("deadlines"),
			Deadlines: testDeadlines(),
			Overdue:   []*model.Deadline{testDeadlines()[1]},
		}},
		{"documents", "documents", documentsData{
			baseData:  base("documents"),
			Documents: testDocuments(),
		}},
		{"generate", "generate", generateData{
			baseData:  base("generate"),
			Templates: testTemplates(),
		}},
		{"subscriptions", "subscriptions", subscriptionsData{
			baseData:   base("subscriptions"),
			Categories: availableCategories,
		}},
	}
}

// TestRenderAuthenticatedPages проверяет, что каждая авторизованная страница
// рендерится целиком (layout + header + nav), а не как обрезанный фрагмент,
// и содержит ожидаемые данные. Это страж против регрессии «headless-фрагментов»
// и обращений к несуществующим полям шаблона.
func TestRenderAuthenticatedPages(t *testing.T) {
	for _, tc := range authPages() {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := portalTmpl.ExecuteTemplate(&buf, tc.tmpl, tc.data); err != nil {
				t.Fatalf("ExecuteTemplate(%q) вернул ошибку: %v", tc.tmpl, err)
			}
			out := buf.String()
			if !strings.Contains(out, "</html>") {
				t.Errorf("страница %q не содержит </html> (вероятно, layout не применён)", tc.tmpl)
			}
			if !strings.Contains(out, "<nav") {
				t.Errorf("страница %q не содержит <nav (меню навигации отсутствует)", tc.tmpl)
			}
			if !strings.Contains(out, "resident@example.ru") {
				t.Errorf("страница %q не содержит email аккаунта в шапке", tc.tmpl)
			}
			// Метка стадии рендерится только на дашборде («Текущая стадия»).
			if tc.tmpl == "dashboard" && !strings.Contains(out, "Резидент") {
				t.Errorf("страница %q не содержит человекочитаемую метку стадии «Резидент»", tc.tmpl)
			}
		})
	}
}

// TestRenderEmptyData проверяет, что страницы с пустыми данными рендерят
// пустые состояния без ошибок и тоже остаются полноценными HTML-страницами.
func TestRenderEmptyData(t *testing.T) {
	empty := []pageCase{
		{"dashboard", "dashboard", dashboardData{baseData: baseData{Client: testClient(), Page: "dashboard"}}},
		{"checklists", "checklists", checklistsData{baseData: baseData{Client: testClient(), Page: "checklists"}}},
		{"deadlines", "deadlines", deadlinesData{baseData: baseData{Client: testClient(), Page: "deadlines"}}},
		{"documents", "documents", documentsData{baseData: baseData{Client: testClient(), Page: "documents"}}},
		{"generate", "generate", generateData{baseData: baseData{Client: testClient(), Page: "generate"}}},
		{"subscriptions", "subscriptions", subscriptionsData{baseData: baseData{Client: testClient(), Page: "subscriptions"}}},
	}
	for _, tc := range empty {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := portalTmpl.ExecuteTemplate(&buf, tc.tmpl, tc.data); err != nil {
				t.Fatalf("ExecuteTemplate(%q) с пустыми данными вернул ошибку: %v", tc.tmpl, err)
			}
			out := buf.String()
			if !strings.Contains(out, "</html>") {
				t.Errorf("пустая страница %q не содержит </html>", tc.tmpl)
			}
			if !strings.Contains(out, "<nav") {
				t.Errorf("пустая страница %q не содержит <nav", tc.tmpl)
			}
		})
	}
}

// TestRenderLogin проверяет, что страница входа рендерится как самостоятельная
// полноценная HTML-страница (её не оборачивают в layout портала).
func TestRenderLogin(t *testing.T) {
	var buf bytes.Buffer
	data := loginData{Flash: "Введите email", FlashKind: "err"}
	if err := portalTmpl.ExecuteTemplate(&buf, "login", data); err != nil {
		t.Fatalf("ExecuteTemplate(login) вернул ошибку: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "</html>") {
		t.Error("страница login не содержит </html>")
	}
	if !strings.Contains(out, "Личный кабинет резидента") {
		t.Error("страница login не содержит заголовок формы входа")
	}
}
