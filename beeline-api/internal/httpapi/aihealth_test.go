package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/nearai"
)

type scriptedModel struct {
	reply string
	err   error
}

func (m scriptedModel) Complete(context.Context, string, string, string) (string, error) {
	return m.reply, m.err
}

func aiHealthFor(t *testing.T, c scriptedModel, attach bool) map[string]any {
	t.Helper()
	aiHealth.body = nil // the answer is cached for a minute; each case starts fresh
	d := Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), ChatModel: "test-model"}
	if attach {
		d.Chat = c
	}
	rec := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health/ai", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAIHealthSaysWhatIsWrongInPlainWords(t *testing.T) {
	if o := aiHealthFor(t, scriptedModel{}, false); o["configured"] != false || o["ok"] != false {
		t.Fatalf("no model attached: %v", o)
	}
	if o := aiHealthFor(t, scriptedModel{reply: "ok"}, true); o["ok"] != true || o["configured"] != true {
		t.Fatalf("healthy: %v", o)
	}
	for status, want := range map[int]string{401: "rejected the API key", 402: "out of credits", 404: "does not know this model", 500: "HTTP 500"} {
		o := aiHealthFor(t, scriptedModel{err: &nearai.Error{Status: status, Message: "boom"}}, true)
		if o["ok"] != false || !strings.Contains(o["detail"].(string), want) {
			t.Fatalf("status %d: %v", status, o)
		}
	}
	if o := aiHealthFor(t, scriptedModel{err: context.DeadlineExceeded}, true); !strings.Contains(o["detail"].(string), "in time") {
		t.Fatalf("timeout: %v", o)
	}
}
