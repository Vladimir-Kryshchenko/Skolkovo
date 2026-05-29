package diff

import (
	"strings"
	"testing"
)

func TestCompareDocuments_Identical(t *testing.T) {
	text := "line1\nline2\nline3"
	result := CompareDocuments(text, text)

	if result.Summary.TotalAdded != 0 {
		t.Errorf("expected 0 added, got %d", result.Summary.TotalAdded)
	}
	if result.Summary.TotalRemoved != 0 {
		t.Errorf("expected 0 removed, got %d", result.Summary.TotalRemoved)
	}
	if result.Summary.TotalModified != 0 {
		t.Errorf("expected 0 modified, got %d", result.Summary.TotalModified)
	}
}

func TestCompareDocuments_AddedLines(t *testing.T) {
	text1 := "line1\nline3"
	text2 := "line1\nline2\nline3"
	result := CompareDocuments(text1, text2)

	if result.Summary.TotalAdded != 1 {
		t.Errorf("expected 1 added, got %d", result.Summary.TotalAdded)
	}
	if result.Summary.TotalRemoved != 0 {
		t.Errorf("expected 0 removed, got %d", result.Summary.TotalRemoved)
	}
}

func TestCompareDocuments_RemovedLines(t *testing.T) {
	text1 := "line1\nline2\nline3"
	text2 := "line1\nline3"
	result := CompareDocuments(text1, text2)

	if result.Summary.TotalRemoved != 1 {
		t.Errorf("expected 1 removed, got %d", result.Summary.TotalRemoved)
	}
	if result.Summary.TotalAdded != 0 {
		t.Errorf("expected 0 added, got %d", result.Summary.TotalAdded)
	}
}

func TestCompareDocuments_Modified(t *testing.T) {
	text1 := "old line\nkept"
	text2 := "new line\nkept"
	result := CompareDocuments(text1, text2)

	if result.Summary.TotalRemoved < 1 {
		t.Error("expected at least 1 removed for modified text")
	}
	if result.Summary.TotalAdded < 1 {
		t.Error("expected at least 1 added for modified text")
	}
}

func TestCompareDocuments_EmptyToNonEmpty(t *testing.T) {
	text1 := ""
	text2 := "hello\nworld"
	result := CompareDocuments(text1, text2)

	if result.Summary.TotalAdded != 2 {
		t.Errorf("expected 2 added, got %d", result.Summary.TotalAdded)
	}
}

func TestCompareDocuments_NonEmptyToEmpty(t *testing.T) {
	text1 := "hello\nworld"
	text2 := ""
	result := CompareDocuments(text1, text2)

	if result.Summary.TotalRemoved != 2 {
		t.Errorf("expected 2 removed, got %d", result.Summary.TotalRemoved)
	}
}

func TestCompareDocuments_MultipleChanges(t *testing.T) {
	text1 := "A\nB\nC\nD\nE"
	text2 := "A\nX\nC\nY\nE"
	result := CompareDocuments(text1, text2)

	if result.Summary.TotalAdded == 0 {
		t.Error("expected some added lines")
	}
	if result.Summary.TotalRemoved == 0 {
		t.Error("expected some removed lines")
	}
}

func TestDiffSection_Grouping(t *testing.T) {
	text1 := "section1\nold_value\nsection2\nold_value2"
	text2 := "section1\nnew_value\nsection2\nnew_value2"
	result := CompareDocuments(text1, text2)

	// Изменения должны быть сгруппированы в секции.
	if result.Summary.TotalModified < 1 {
		t.Error("expected at least 1 modified section")
	}

	for _, sec := range result.ModifiedSections {
		if sec.Title == "" {
			t.Error("section title should not be empty")
		}
		if len(sec.Changes) == 0 {
			t.Error("section should have at least one change")
		}
	}
}

func TestToHTML_NotEmpty(t *testing.T) {
	text1 := "old"
	text2 := "new"
	result := CompareDocuments(text1, text2)
	html := ToHTML(result)

	if html == "" {
		t.Fatal("ToHTML returned empty string")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("ToHTML should produce valid HTML document")
	}
	if !strings.Contains(html, "removed") && !strings.Contains(html, "added") {
		t.Error("ToHTML should contain added or removed classes")
	}
}

func TestToHTML_Identical(t *testing.T) {
	text := "same"
	result := CompareDocuments(text, text)
	html := ToHTML(result)

	if !strings.Contains(html, "+0") || !strings.Contains(html, "-0") {
		t.Error("ToHTML summary should show zero changes for identical texts")
	}
}

func TestToHTML_Escaping(t *testing.T) {
	text1 := "<script>alert('xss')</script>"
	text2 := "safe text"
	result := CompareDocuments(text1, text2)
	html := ToHTML(result)

	if strings.Contains(html, "<script>") {
		t.Error("ToHTML should escape HTML special characters")
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a\nb", []string{"a", "b"}},
		{"a\nb\n", []string{"a", "b"}}, // trailing newline stripped
	}

	for _, tc := range tests {
		got := splitLines(tc.input)
		if len(got) != len(tc.expected) {
			t.Errorf("splitLines(%q) = %v, want %v", tc.input, got, tc.expected)
			continue
		}
		for i := range got {
			if got[i] != tc.expected[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.expected[i])
			}
		}
	}
}

func TestChangeType_Constants(t *testing.T) {
	if ChangeAdded != "added" {
		t.Errorf("ChangeAdded = %q, want %q", ChangeAdded, "added")
	}
	if ChangeRemoved != "removed" {
		t.Errorf("ChangeRemoved = %q, want %q", ChangeRemoved, "removed")
	}
	if ChangeModified != "modified" {
		t.Errorf("ChangeModified = %q, want %q", ChangeModified, "modified")
	}
}
