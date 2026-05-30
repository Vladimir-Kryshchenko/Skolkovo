package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"baza-skolkovo/src/common/model"
)

// JSONStore — файловый реестр документов в формате
// {"documents": [...]} (см. реестр_документов.schema.json).
type JSONStore struct {
	path string
	mu   sync.RWMutex
	docs map[string]model.Document
}

type registryFile struct {
	Documents []model.Document `json:"documents"`
}

// NewJSONStore открывает (или создаёт) файловый реестр по указанному пути.
func NewJSONStore(path string) (*JSONStore, error) {
	s := &JSONStore{path: path, docs: map[string]model.Document{}}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, s.flush()
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	var rf registryFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, err
	}
	for _, d := range rf.Documents {
		s.docs[d.ID] = d
	}
	return s, nil
}

// flush сериализует текущее состояние на диск. Вызывающий держит s.mu.
func (s *JSONStore) flush() error {
	rf := registryFile{Documents: make([]model.Document, 0, len(s.docs))}
	for _, d := range s.docs {
		rf.Documents = append(rf.Documents, d)
	}
	sort.Slice(rf.Documents, func(i, j int) bool { return rf.Documents[i].ID < rf.Documents[j].ID })
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *JSONStore) Upsert(_ context.Context, doc model.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[doc.ID] = doc
	return s.flush()
}

func (s *JSONStore) Get(_ context.Context, id string) (model.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[id]
	if !ok {
		return model.Document{}, ErrNotFound
	}
	return d, nil
}

func (s *JSONStore) List(_ context.Context, f Filter) ([]model.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Document, 0, len(s.docs))
	for _, d := range s.docs {
		if match(d, f) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FetchedAt.After(out[j].FetchedAt) })
	return out, nil
}

func (s *JSONStore) SetStatus(_ context.Context, id string, st model.Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok {
		return ErrNotFound
	}
	d.Status = st
	s.docs[id] = d
	return s.flush()
}

func (s *JSONStore) SetIndexed(_ context.Context, id string, indexed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok {
		return ErrNotFound
	}
	d.Indexed = indexed
	s.docs[id] = d
	return s.flush()
}

func (s *JSONStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.docs[id]; !ok {
		return ErrNotFound
	}
	delete(s.docs, id)
	return s.flush()
}

func (s *JSONStore) Close() error { return nil }
