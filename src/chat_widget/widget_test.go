package widget

import "testing"

func TestExtractMCPJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"обычный JSON", `{"result":{"x":1}}`, `{"result":{"x":1}}`},
		{"JSON с пробелами", "  {\"a\":1}\n", `{"a":1}`},
		{"SSE one event", "event: message\ndata: {\"result\":1}\n\n", `{"result":1}`},
		{"SSE берёт последний data", "data: {\"a\":1}\n\ndata: {\"b\":2}\n", `{"b":2}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(extractMCPJSON([]byte(tt.in))); got != tt.want {
				t.Errorf("extractMCPJSON() = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}
