package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/nearai"
)

// GET /api/v1/health/ai pings the model behind Bee with the server's own key, so a missing key, a rejected
// key, no credits or a retired model shows up as one readable line instead of a silent fallback. The key
// never leaves the server. The answer is cached for a minute, so polling it costs at most one tiny
// completion a minute.

var aiHealth struct {
	mu   sync.Mutex
	at   time.Time
	body map[string]any
}

func (h *handlers) healthAI(w http.ResponseWriter, r *http.Request) {
	aiHealth.mu.Lock()
	defer aiHealth.mu.Unlock()
	if aiHealth.body != nil && time.Since(aiHealth.at) < time.Minute {
		writeJSON(w, http.StatusOK, aiHealth.body)
		return
	}
	out := map[string]any{"model": h.d.ChatModel, "checked_at": time.Now().UTC()}
	switch {
	case h.d.Chat == nil:
		out["configured"], out["ok"] = false, false
		out["detail"] = "No model is attached: NEAR_AI_API_KEY is not set on this server, or the playground is not configured. Bee uses the basic command parser."
	default:
		out["configured"] = true
		ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
		defer cancel()
		start := time.Now()
		reply, err := h.d.Chat.Complete(ctx, h.d.ChatModel, "Reply with the single word ok.", "ping")
		out["latency_ms"] = time.Since(start).Milliseconds()
		if err != nil {
			out["ok"] = false
			out["detail"] = describeAIError(err)
		} else {
			out["ok"] = true
			out["detail"] = "The model answered."
			out["sample"] = strings.TrimSpace(clipRunes(reply, 40))
		}
	}
	aiHealth.at, aiHealth.body = time.Now(), out
	writeJSON(w, http.StatusOK, out)
}

// describeAIError turns a model failure into one safe line (provider status and message only).
func describeAIError(err error) string {
	var pe *nearai.Error
	if errors.As(err, &pe) {
		switch pe.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "NEAR AI rejected the API key (HTTP " + itoa(pe.Status) + "): " + pe.Message
		case http.StatusPaymentRequired:
			return "NEAR AI says the account is out of credits (HTTP 402): " + pe.Message
		case http.StatusNotFound:
			return "NEAR AI does not know this model (HTTP 404): " + pe.Message
		}
		return "NEAR AI returned HTTP " + itoa(pe.Status) + ": " + pe.Message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "NEAR AI did not answer in time."
	}
	return "Could not reach NEAR AI."
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
