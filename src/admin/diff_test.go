package admin

import (
	"testing"
	"time"

	"baza-skolkovo/src/changes"
)

// TestLatestVerdicts проверяет выбор «последнего вердикта на документ» из ленты
// изменений: берётся самое свежее событие с непустой важностью; документы без
// вердиктов не попадают в карту; устаревший вердикт используется, если самое
// свежее событие документа ещё не проанализировано.
func TestLatestVerdicts(t *testing.T) {
	now := time.Now()
	// События отсортированы по убыванию времени — как их отдаёт Recent.
	evs := []changes.Event{
		// docA: самое свежее событие БЕЗ вердикта → пропускаем, берём более
		// старое проанализированное.
		{ID: "1", EntityType: changes.EntityDocument, EntityID: "docA", DetectedAt: now, Severity: ""},
		{ID: "2", EntityType: changes.EntityDocument, EntityID: "docA", DetectedAt: now.Add(-1 * time.Hour), Severity: changes.SeverityWarning, AnalysisSummary: "вердикт A"},
		{ID: "3", EntityType: changes.EntityDocument, EntityID: "docA", DetectedAt: now.Add(-2 * time.Hour), Severity: changes.SeverityInfo, AnalysisSummary: "ещё старее A"},
		// docB: самое свежее уже с вердиктом → берём его.
		{ID: "4", EntityType: changes.EntityDocument, EntityID: "docB", DetectedAt: now.Add(-30 * time.Minute), Severity: changes.SeverityCritical, AnalysisSummary: "свежий B"},
		{ID: "5", EntityType: changes.EntityDocument, EntityID: "docB", DetectedAt: now.Add(-3 * time.Hour), Severity: changes.SeverityInfo, AnalysisSummary: "старый B"},
		// docC: без вердиктов вовсе → не попадает в карту.
		{ID: "6", EntityType: changes.EntityDocument, EntityID: "docC", DetectedAt: now, Severity: ""},
	}

	got := latestVerdicts(evs)

	if len(got) != 2 {
		t.Fatalf("ожидалось 2 документа с вердиктом, получено %d: %+v", len(got), got)
	}
	a, ok := got["docA"]
	if !ok {
		t.Fatal("docA должен быть в карте (есть более старое проанализированное событие)")
	}
	if a.ID != "2" || a.Severity != changes.SeverityWarning {
		t.Errorf("docA: ожидалось событие 2 (warning), получено %s (%s)", a.ID, a.Severity)
	}
	if b := got["docB"]; b.ID != "4" || b.Severity != changes.SeverityCritical {
		t.Errorf("docB: ожидалось самое свежее событие 4 (critical), получено %s (%s)", b.ID, b.Severity)
	}
	if _, ok := got["docC"]; ok {
		t.Error("docC не должен попадать в карту — у него нет вердиктов")
	}
}
