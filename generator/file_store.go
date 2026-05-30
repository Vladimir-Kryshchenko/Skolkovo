package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
)

// FileTemplateStore — реализация TemplateStore поверх директории с файлами шаблонов.
// Идентификатор шаблона = имя файла (например «Заявление_на_резидентство.go.tpl»).
// Используется генератором, когда шаблоны лежат на диске (созданы CreateDefaultTemplates).
type FileTemplateStore struct {
	Dir string
}

// NewFileTemplateStore создаёт файловое хранилище шаблонов.
func NewFileTemplateStore(dir string) *FileTemplateStore {
	return &FileTemplateStore{Dir: dir}
}

// templateName убирает шаблонные расширения, оставляя человекочитаемое имя.
func templateName(file string) string {
	for _, ext := range []string{".go.tpl", ".docx.tpl", ".html.tpl"} {
		if strings.HasSuffix(strings.ToLower(file), ext) {
			return strings.TrimSuffix(file, file[len(file)-len(ext):])
		}
	}
	return strings.TrimSuffix(file, filepath.Ext(file))
}

// ListTemplates перечисляет файлы шаблонов в директории.
func (f *FileTemplateStore) ListTemplates(_ context.Context) ([]model.DocumentTemplate, error) {
	entries, err := os.ReadDir(f.Dir)
	if err != nil {
		return nil, fmt.Errorf("FileTemplateStore: чтение %q: %w", f.Dir, err)
	}
	var out []model.DocumentTemplate
	for _, e := range entries {
		if e.IsDir() || !strings.Contains(strings.ToLower(e.Name()), ".tpl") {
			continue
		}
		out = append(out, model.DocumentTemplate{
			ID:           e.Name(),
			Name:         templateName(e.Name()),
			TemplateFile: e.Name(),
			Type:         strings.TrimPrefix(templateExtension(e.Name()), "."),
			Version:      "1.0",
			CreatedAt:    time.Now(),
		})
	}
	return out, nil
}

// GetTemplate возвращает шаблон по идентификатору (имени файла).
func (f *FileTemplateStore) GetTemplate(ctx context.Context, templateID string) (*model.DocumentTemplate, error) {
	list, err := f.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == templateID {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("FileTemplateStore: шаблон %q не найден в %q", templateID, f.Dir)
}

// ReadTemplateFile читает файл шаблона из директории.
func (f *FileTemplateStore) ReadTemplateFile(_ context.Context, templateFile string) ([]byte, error) {
	// Защита от выхода за пределы директории шаблонов.
	clean := filepath.Base(templateFile)
	return os.ReadFile(filepath.Join(f.Dir, clean))
}
