// Package guard is the AI fraud analyst. Whenever something notable happens to an invoice (a payment claim is
// refused, a payment is confirmed, a payment from another chain is refunded) the server gathers the facts itself
// and asks a NEAR AI model for a verdict and a plain-language reason, stored as evidence for the vendor.
//
// What the agent can and cannot do:
//   - It sees only facts the SERVER computed (expected vs observed token, amount, payee, memo, counts of earlier
//     attempts). It never sees, and cannot be talked to by, the customer.
//   - It cannot mark an invoice paid or unpaid, release or hold money, or change any other state. The chain
//     verifier and NEAR Intents decide payment; Guard advises.
//   - A refused claim or a refund is never rated "clear": the verdict is floored at "watch" whatever the model says.
//   - If the model is unavailable, no review is written. Nothing blocks on Guard.
package guard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Completer runs one chat completion (satisfied by *nearai.Client).
type Completer interface {
	Complete(ctx context.Context, model, system, user string) (string, error)
}

// Subjects of a review.
const (
	SubjectClaimRefused     = "claim_refused"
	SubjectPaymentConfirmed = "payment_confirmed"
	SubjectPayinEvent       = "payin_event"
)

// Verdicts.
const (
	VerdictClear = "clear"
	VerdictWatch = "watch"
	VerdictFraud = "fraud"
)

// Event is what happened, with facts computed by the server.
type Event struct {
	InvoiceID string
	Subject   string
	Ref       string            // transaction hash or deposit address
	Facts     map[string]string // short, server-computed facts
}

// Review is a stored verdict.
type Review struct {
	ID        int64             `json:"id"`
	InvoiceID string            `json:"invoice_id"`
	Subject   string            `json:"subject"`
	Ref       string            `json:"ref"`
	Verdict   string            `json:"verdict"`
	Reason    string            `json:"reason"`
	Model     string            `json:"model"`
	Evidence  map[string]string `json:"evidence"`
	CreatedAt time.Time         `json:"created_at"`
}

// Service reviews events and stores the verdicts.
type Service struct {
	DB    *pgxpool.Pool
	LLM   Completer
	Model string
}

const systemPrompt = `You are Guard, the fraud analyst inside Beeline, a point of sale on NEAR. A server hands you facts about ONE payment event on an invoice. The facts were computed by the server from the chain and the invoice; they are not written by the customer. Decide how suspicious the event is for the vendor.

Verdicts:
- "clear": nothing suspicious. Only for confirmed payments with consistent facts.
- "watch": something is off or unusual, but it may be an honest mistake (wrong amount, wrong network, a single refused attempt, a refund).
- "fraud": the pattern looks like deliberate fraud (look-alike token, edited payee, replayed transaction, repeated attempts with changing details, claiming someone else's transaction).

Rules:
- "fraud" needs strong evidence: three or more refused attempts on the same invoice, a look-alike or fake token, an edited payee, or a replayed transaction. One or two refused attempts with a wrong sender, amount or network are "watch": people make honest mistakes.
- Judge only from the facts. Do not invent facts. If the facts are thin, say "watch" or "clear", not "fraud".
- Reply with ONE JSON object and nothing else: {"verdict":"clear|watch|fraud","reason":"one or two plain sentences for a shop owner, no jargon"}.
- You cannot move money or mark anything paid. You are advising the vendor.`

var jsonObjRe = regexp.MustCompile(`(?s)\{.*\}`)

// Parse reads the model's answer. The verdict must be one of the three; the reason is clipped.
func Parse(out string) (verdict, reason string, err error) {
	m := jsonObjRe.FindString(out)
	if m == "" {
		return "", "", errors.New("no JSON in guard output")
	}
	var v struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(m), &v); err != nil {
		return "", "", err
	}
	v.Verdict = strings.ToLower(strings.TrimSpace(v.Verdict))
	switch v.Verdict {
	case VerdictClear, VerdictWatch, VerdictFraud:
	default:
		return "", "", fmt.Errorf("unknown verdict %q", v.Verdict)
	}
	reason = strings.TrimSpace(v.Reason)
	if reason == "" {
		return "", "", errors.New("empty reason")
	}
	if r := []rune(reason); len(r) > 300 {
		reason = string(r[:300])
	}
	return v.Verdict, reason, nil
}

// floor applies the rules the model cannot override.
func floor(subject, verdict string) string {
	if subject != SubjectPaymentConfirmed && verdict == VerdictClear {
		return VerdictWatch // a refused claim or a refund is never "clear"
	}
	return verdict
}

// Review asks the model about one event and stores the verdict. A repeat of the same event returns the stored one.
func (s *Service) Review(ctx context.Context, ev Event) (*Review, error) {
	if s == nil || s.LLM == nil {
		return nil, errors.New("guard is not configured")
	}
	if cur, err := s.get(ctx, ev); err == nil {
		return cur, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	facts, _ := json.Marshal(ev.Facts)
	out, err := s.LLM.Complete(ctx, s.Model, systemPrompt, fmt.Sprintf("Event: %s\nFacts: %s", ev.Subject, facts))
	if err != nil {
		return nil, err
	}
	verdict, reason, err := Parse(out)
	if err != nil {
		return nil, err
	}
	verdict = floor(ev.Subject, verdict)
	_, err = s.DB.Exec(ctx, `INSERT INTO guard_reviews (invoice_id, subject, ref, verdict, reason, model, evidence)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (invoice_id, subject, ref) DO NOTHING`,
		ev.InvoiceID, ev.Subject, ev.Ref, verdict, reason, s.Model, facts)
	if err != nil {
		return nil, err
	}
	return s.get(ctx, ev)
}

func (s *Service) get(ctx context.Context, ev Event) (*Review, error) {
	return scan(s.DB.QueryRow(ctx, `SELECT id, invoice_id, subject, ref, verdict, reason, model, evidence, created_at
		FROM guard_reviews WHERE invoice_id = $1 AND subject = $2 AND ref = $3`, ev.InvoiceID, ev.Subject, ev.Ref))
}

func scan(row pgx.Row) (*Review, error) {
	var r Review
	var ev []byte
	if err := row.Scan(&r.ID, &r.InvoiceID, &r.Subject, &r.Ref, &r.Verdict, &r.Reason, &r.Model, &ev, &r.CreatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(ev, &r.Evidence)
	return &r, nil
}

// ForInvoice lists an invoice's reviews, newest first.
func (s *Service) ForInvoice(ctx context.Context, invoiceID string, limit int) ([]Review, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, invoice_id, subject, ref, verdict, reason, model, evidence, created_at
		FROM guard_reviews WHERE invoice_id = $1 ORDER BY id DESC LIMIT $2`, invoiceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Review{}
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}
