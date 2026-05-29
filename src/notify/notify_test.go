package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNotifierDisabled(t *testing.T) {
	if New("").Enabled() {
		t.Error("пустой URL должен отключать нотификатор")
	}
	// Send на отключённом не должен падать.
	if err := New("").Send(context.Background(), Event{}); err != nil {
		t.Errorf("Send на отключённом вернул ошибку: %v", err)
	}
}

func TestNotifierSend(t *testing.T) {
	var got Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.URL)
	ev := Event{Type: "test", Timestamp: time.Now(), Message: "привет"}
	if err := n.Send(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if got.Type != "test" || got.Message != "привет" {
		t.Errorf("webhook получил неверное событие: %+v", got)
	}
}
