package changes

import (
	"context"
	"testing"
)

// fakeRecorder фиксирует переданные события в память.
type fakeRecorder struct {
	events []Event
	err    error
}

func (f *fakeRecorder) Record(_ context.Context, ev Event) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, ev)
	return nil
}

func TestNotifySkipsNil(t *testing.T) {
	rec := &fakeRecorder{}
	ev := Event{EntityType: EntityDocument, EntityID: "doc-1", Title: "Регламент", Kind: KindNew}

	// nil-рекордеры в списке должны игнорироваться, не вызывая паники.
	Notify(context.Background(), []Recorder{nil, rec, nil}, ev)

	if len(rec.events) != 1 {
		t.Fatalf("ожидалось 1 событие, получено %d", len(rec.events))
	}
	if rec.events[0].EntityID != "doc-1" {
		t.Errorf("EntityID = %q, ожидалось doc-1", rec.events[0].EntityID)
	}
}

func TestNotifyEmptyRecorders(t *testing.T) {
	// Пустой список и nil-список не должны паниковать.
	Notify(context.Background(), nil, Event{EntityID: "x"})
	Notify(context.Background(), []Recorder{}, Event{EntityID: "x"})
}

func TestNotifyFanOut(t *testing.T) {
	a, b := &fakeRecorder{}, &fakeRecorder{}
	Notify(context.Background(), []Recorder{a, b}, Event{EntityID: "doc-2", Kind: KindUpdated})
	if len(a.events) != 1 || len(b.events) != 1 {
		t.Fatalf("событие должно попасть в оба рекордера: a=%d b=%d", len(a.events), len(b.events))
	}
}
