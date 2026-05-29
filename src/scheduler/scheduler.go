// Package scheduler хранит настройки планировщика и отчёты.
package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"baza-skolkovo/src/common/model"
)

// Store хранит настройки планировщика.
type Store struct {
	path string
	mu   sync.RWMutex
	cfg  model.SchedulerSettings
}

// New создаёт хранилище настроек.
func New(dataDir string) (*Store, error) {
	path := filepath.Join(dataDir, "scheduler_settings.json")
	s := &Store{path: path}

	// Создаём дефолтные настройки если файл не существует
	if _, err := os.Stat(path); os.IsNotExist(err) {
		s.cfg = model.SchedulerSettings{
			Enabled:      true,
			IntervalDays: 3,
			AutoApprove:  false,
			AutoIndex:    true,
			AutoValidate: true,
		}
		if err := s.Save(); err != nil {
			return nil, fmt.Errorf("создание настроек: %w", err)
		}
		return s, nil
	}

	if err := s.Load(); err != nil {
		return nil, fmt.Errorf("загрузка настроек: %w", err)
	}
	return s, nil
}

// Load загружает настройки из файла.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.cfg)
}

// Save сохраняет настройки в файл.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// GetSettings возвращает текущие настройки.
func (s *Store) GetSettings() model.SchedulerSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// UpdateSettings обновляет настройки.
func (s *Store) UpdateSettings(updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Применяем обновления
	if v, ok := updates["enabled"].(bool); ok {
		s.cfg.Enabled = v
	}
	if v, ok := updates["interval_days"].(float64); ok {
		s.cfg.IntervalDays = int(v)
		if s.cfg.IntervalDays < 1 {
			s.cfg.IntervalDays = 1
		}
	}
	if v, ok := updates["auto_approve"].(bool); ok {
		s.cfg.AutoApprove = v
	}
	if v, ok := updates["auto_index"].(bool); ok {
		s.cfg.AutoIndex = v
	}
	if v, ok := updates["auto_validate"].(bool); ok {
		s.cfg.AutoValidate = v
	}

	// Пересчитываем NextRun
	if s.cfg.Enabled {
		now := time.Now()
		if s.cfg.LastRun != nil {
			next := s.cfg.LastRun.AddDate(0, 0, s.cfg.IntervalDays)
			s.cfg.NextRun = &next
		} else {
			next := now.AddDate(0, 0, s.cfg.IntervalDays)
			s.cfg.NextRun = &next
		}
	} else {
		s.cfg.NextRun = nil
	}

	// Сохраняем
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// MarkRun записывает время запуска.
func (s *Store) MarkRun() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.cfg.LastRun = &now
	next := now.AddDate(0, 0, s.cfg.IntervalDays)
	s.cfg.NextRun = &next

	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// ShouldRun проверяет, пора ли запускать сбор.
func (s *Store) ShouldRun() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.cfg.Enabled {
		return false
	}
	if s.cfg.NextRun == nil {
		return true // первый запуск
	}
	return time.Now().After(*s.cfg.NextRun)
}

// ReportStore хранит отчёты о сборе.
type ReportStore struct {
	path string
	mu   sync.RWMutex
}

// NewReportStore создаёт хранилище отчётов.
func NewReportStore(dataDir string) (*ReportStore, error) {
	path := filepath.Join(dataDir, "collector_reports.json")
	rs := &ReportStore{path: path}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
			return nil, err
		}
	}
	return rs, nil
}

// AddReport добавляет отчёт.
func (rs *ReportStore) AddReport(rep model.CollectorReport) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	reports, err := rs.loadLocked()
	if err != nil {
		return err
	}
	reports = append(reports, rep)
	// Храним только последние 50 отчётов
	if len(reports) > 50 {
		reports = reports[len(reports)-50:]
	}
	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(rs.path, data, 0o644)
}

// GetReports возвращает последние отчёты.
func (rs *ReportStore) GetReports(limit int) ([]model.CollectorReport, error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	reports, err := rs.loadLocked()
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(reports) > limit {
		reports = reports[len(reports)-limit:]
	}
	return reports, nil
}

func (rs *ReportStore) loadLocked() ([]model.CollectorReport, error) {
	data, err := os.ReadFile(rs.path)
	if err != nil {
		return nil, err
	}
	var reports []model.CollectorReport
	if err := json.Unmarshal(data, &reports); err != nil {
		return nil, err
	}
	return reports, nil
}
