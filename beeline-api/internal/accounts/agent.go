package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

// Bee is the signed-in user's assistant. It is an agent in the plain sense: each turn it reads the
// conversation, may call a tool, reads the result, and answers. The model runs on NEAR AI Cloud.
//
// The model's output is UNTRUSTED. It can only ask for the tools below, and the server checks every
// argument itself:
//   - it can never pay anything or move money, and it cannot choose who gets paid: invoices always
//     pay the user's saved payout account;
//   - set_profile only takes a shop name and account the user literally typed in their latest
//     message (or "wallet", meaning the wallet created for them), so a model cannot redirect payouts;
//   - prices are exact decimal strings converted to integer base units, never floats, and the
//     real-money cap (1.5 USDC in early access) still applies.

// Completer runs one chat completion (satisfied by *nearai.Client).
type Completer interface {
	Complete(ctx context.Context, model, system, user string) (string, error)
}

// Host is what the agent's tools act on. The HTTP layer implements it for one signed-in user.
type Host interface {
	Profile() (shop, account string)
	SetProfile(ctx context.Context, shop, account string) error
	CreateInvoice(ctx context.Context, items []invoices.Item) (*invoices.Invoice, error)
	ListInvoices(ctx context.Context) ([]invoices.Invoice, error)
	PermanentLinkPath() (string, error)
	// NGNRate is naira per 1 USDC (decimal string) from Switch's payout quote, used to convert naira prices.
	NGNRate(ctx context.Context) (string, error)
}

// Result is what one turn produced.
type Result struct {
	Reply         string
	Invoice       *invoices.Invoice // the last invoice made this turn, if any
	PermanentPath string            // a /pay/to?... path, if the permanent link was requested
	NeedsProfile  bool
}

const maxSteps = 4

const systemPrompt = `You are Bee, the payment assistant inside Beeline, a point of sale on NEAR. Be warm, brief and practical.

What you can do for the signed-in user (and nothing else):
- set up their shop: a shop name and the NEAR account that receives their money
- make an invoice (a signed pay link they can share on WhatsApp, Facebook, X, Telegram, LinkedIn, email, SMS, or show as a QR at the counter)
- give them one permanent pay link to keep, where each customer chooses the amount
- list their invoices and say which are paid
- explain how Beeline protects them: a NEAR vault pays only the exact payee and amount on the signed invoice, once, and blocks anything else
- point them to "Cash out to bank" in the sidebar to turn USDC into naira (you cannot do that step yourself)

Rules:
- Prices are in USDC unless the user says naira (NGN). If a price has no currency and looks like naira (over 1000), ask "Is that in naira or USDC?" before making the invoice. Never convert currencies yourself: pass the naira price with "currency":"NGN" and the app converts at Switch's rate and tells you the result.
- Every invoice is REAL money: USDC on NEAR mainnet, at most 1.5 USDC each while Beeline is in early access. Customers pay by scanning a QR code with any wallet or exchange (Binance, Bybit, Tokkenly, MetaMask and more, on many chains) or from a NEAR wallet. There is no test or demo invoice here: if asked, say invoices are real money. Prices must be exact numbers the user gave you; never invent a price or guess.
- You cannot send, pay, refund or move money. Never say a payment was made unless list_invoices shows status "paid".
- If you need shop name or payout account, ask for it. If the user says to use their wallet, call set_profile with near_account "wallet".
- You are not a financial advisor. If asked something unrelated, say briefly what you can help with.

You reply with ONE JSON object and nothing else.
To use a tool: {"tool":"<name>","args":{...}}
To answer the user: {"say":"<your message>"}

Tools:
- set_profile {"shop_name":"...","near_account":"... or wallet"}   (only values the user typed in their last message)
- create_invoice {"items":[{"name":"...","quantity":1,"price":"1.00"}],"currency":"USDC"}   (price is per unit, as a decimal string, in USDC by default; use "currency":"NGN" only when the user's own message says the prices are in naira, and the app converts to USDC)
- permanent_link {}
- list_invoices {}

After a tool result arrives, either call another tool or answer with {"say": ...}. When create_invoice succeeds, say what you made in one or two sentences; the app shows the share buttons.`

type step struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
	Say  string          `json:"say"`
}

var jsonObjRe = regexp.MustCompile(`(?s)\{.*\}`)

func parseStep(out string) (*step, error) {
	m := jsonObjRe.FindString(out)
	if m == "" {
		return nil, errors.New("no JSON in model output")
	}
	var s step
	if err := json.Unmarshal([]byte(m), &s); err != nil {
		return nil, err
	}
	if s.Tool == "" && s.Say == "" {
		return nil, errors.New("empty step")
	}
	return &s, nil
}

// Run answers userMsg. history is the recent conversation, oldest first, not including userMsg.
// wallet is the wallet address the client reports for this user ("" if none yet).
func Run(ctx context.Context, c Completer, model string, history []Message, userMsg, wallet string, host Host) (*Result, error) {
	var tr strings.Builder
	for _, m := range history {
		who := "User"
		if m.Role == "assistant" {
			who = "Bee"
		}
		fmt.Fprintf(&tr, "%s: %s\n", who, clip(m.Body, 400))
	}
	shop, acct := host.Profile()
	if shop == "" || acct == "" {
		tr.WriteString("[app: the user has not set up a shop yet (missing shop name and/or payout account)]\n")
	} else {
		fmt.Fprintf(&tr, "[app: shop %q, payout account %q]\n", shop, acct)
	}
	if wallet != "" {
		tr.WriteString("[app: a NEAR wallet was created for this user; \"wallet\" means that wallet]\n")
	}
	fmt.Fprintf(&tr, "User: %s\n", userMsg)

	res := &Result{}
	for i := 0; i < maxSteps; i++ {
		out, err := c.Complete(ctx, model, systemPrompt, tr.String())
		if err != nil {
			return nil, err
		}
		st, err := parseStep(out)
		if err != nil {
			// A model that ignores the format still gets to answer, as plain text. This holds after a tool call too:
			// falling back to the command parser then would throw away what the tools already did (and could make a
			// second invoice).
			if s := strings.TrimSpace(out); s != "" {
				res.Reply = clip(s, 1200)
				return res, nil
			}
			return nil, err
		}
		if st.Tool == "" {
			res.Reply = clip(st.Say, 1200)
			return res, nil
		}
		obs := runTool(ctx, host, st, userMsg, wallet, res)
		fmt.Fprintf(&tr, "Bee tool call: %s\nTool result: %s\n", clip(st.Tool, 40), obs)
	}
	if res.Reply == "" {
		res.Reply = "I got a bit tangled. Could you say that again in one short sentence?"
	}
	return res, nil
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// runTool executes one tool and returns a short text result for the model. Failures are results,
// not errors: the model can read them and explain.
func runTool(ctx context.Context, host Host, st *step, userMsg, wallet string, res *Result) string {
	switch st.Tool {
	case "set_profile":
		var a struct {
			Shop string `json:"shop_name"`
			Near string `json:"near_account"`
		}
		if json.Unmarshal(st.Args, &a) != nil {
			return "error: bad arguments"
		}
		a.Shop, a.Near = strings.TrimSpace(a.Shop), strings.ToLower(strings.TrimSpace(a.Near))
		low := strings.ToLower(userMsg)
		// The model may only use what the user typed in this message.
		if a.Shop == "" || !strings.Contains(low, strings.ToLower(a.Shop)) {
			return "error: the shop name must be exactly what the user typed in their last message; ask them for it"
		}
		if a.Near == "wallet" {
			if wallet == "" {
				return "error: the user's wallet is not ready yet; ask for a NEAR account instead"
			}
			a.Near = strings.ToLower(wallet)
		} else if !strings.Contains(low, a.Near) {
			return "error: the NEAR account must be exactly what the user typed in their last message; ask them for it"
		}
		if err := host.SetProfile(ctx, a.Shop, a.Near); err != nil {
			return "error: " + err.Error()
		}
		return fmt.Sprintf("saved: shop %q, payout account %q", a.Shop, a.Near)

	case "create_invoice":
		shop, acct := host.Profile()
		if shop == "" || acct == "" {
			res.NeedsProfile = true
			return "error: the user has no shop set up yet; ask for the shop name and payout account first"
		}
		var a struct {
			Items []struct {
				Name     string          `json:"name"`
				Quantity int             `json:"quantity"`
				Price    json.RawMessage `json:"price"`
			} `json:"items"`
			Currency string `json:"currency"`
		}
		if json.Unmarshal(st.Args, &a) != nil || len(a.Items) == 0 || len(a.Items) > 10 {
			return "error: items must be a list of 1 to 10 items"
		}
		cur := strings.ToUpper(strings.TrimSpace(a.Currency))
		if cur == "" {
			cur = "USDC"
		}
		rate := ""
		if cur == "NGN" {
			if !wantsNaira(userMsg) {
				return "error: only price in naira when the user's message says naira; otherwise ask whether the price is in naira or USDC"
			}
			var rerr error
			if rate, rerr = host.NGNRate(ctx); rerr != nil {
				return "error: today's naira rate is not available right now; ask the user to price in USDC or try again shortly"
			}
		} else if cur != "USDC" {
			return "error: currency must be USDC or NGN"
		}
		var items []invoices.Item
		totalUnits := new(big.Int)
		for _, it := range a.Items {
			price := strings.Trim(strings.TrimSpace(string(it.Price)), `"`)
			if cur == "NGN" {
				units, err := ngnToUnits(price, rate)
				if err != nil {
					return fmt.Sprintf("error: %q is not a valid naira price; use a positive amount like 600000", price)
				}
				q := it.Quantity
				if q == 0 {
					q = 1
				}
				name := strings.TrimSpace(it.Name)
				if name == "" {
					name = "Payment"
				}
				name = clip(name, 50) + " (₦" + niceNaira(price) + " each)"
				u, _ := new(big.Int).SetString(units, 10)
				totalUnits.Add(totalUnits, new(big.Int).Mul(u, big.NewInt(int64(q))))
				items = append(items, invoices.Item{Name: name, Quantity: q, UnitPrice: units})
				continue
			}
			units, ok := usdcUnits(price)
			if !ok || !regexp.MustCompile(`^\d+(\.\d{1,6})?$`).MatchString(price) {
				return fmt.Sprintf("error: %q is not a valid price; use a positive decimal like 25 or 0.15", price)
			}
			q := it.Quantity
			if q == 0 {
				q = 1
			}
			name := strings.TrimSpace(it.Name)
			if name == "" {
				name = "Payment"
			}
			items = append(items, invoices.Item{Name: clip(name, 80), Quantity: q, UnitPrice: units})
		}
		inv, err := host.CreateInvoice(ctx, items)
		if err != nil {
			if cur == "NGN" {
				return fmt.Sprintf("error: %s. For reference the converted total is about %s USDC at %s naira per USDC", err.Error(), usdcString(totalUnits.String()), rate)
			}
			return "error: " + err.Error()
		}
		res.Invoice = inv
		if cur == "NGN" {
			return "created invoice " + inv.ID + " for " + usdcString(inv.Amount) + " USDC (converted from naira at " + rate + " naira per USDC, Switch's rate now; the invoice is fixed in USDC) payable to " + inv.Payout + " (status " + inv.Status + ")"
		}
		return "created invoice " + inv.ID + " for " + usdcString(inv.Amount) + " USDC payable to " + inv.Payout + " (status " + inv.Status + ")"

	case "permanent_link":
		p, err := host.PermanentLinkPath()
		if err != nil {
			res.NeedsProfile = true
			return "error: " + err.Error()
		}
		res.PermanentPath = p
		return "permanent link ready; the app shows it with share buttons. Customers choose their own amount (up to 5 test USDC)."

	case "list_invoices":
		list, err := host.ListInvoices(ctx)
		if err != nil {
			return "error: could not read invoices"
		}
		if len(list) == 0 {
			return "the user has no invoices yet"
		}
		var b strings.Builder
		for i, inv := range list {
			if i == 10 {
				break
			}
			fmt.Fprintf(&b, "- %s USDC, status %s, id %s\n", usdcString(inv.Amount), inv.Status, inv.ID)
		}
		return b.String()
	}
	return "error: unknown tool"
}

var realWordRe = regexp.MustCompile(`\b(real|live|mainnet)\b`)

// wantsReal is true only when the user's own message asks for a real-money invoice. The model cannot decide that.
func wantsReal(userMsg string) bool { return realWordRe.MatchString(strings.ToLower(userMsg)) }

// usdcString renders base units as a USDC decimal ("2500000" -> "2.5").
func usdcString(u string) string {
	n, err := strconv.ParseUint(u, 10, 64)
	if err != nil {
		return u
	}
	whole, frac := n/1_000_000, fmt.Sprintf("%06d", n%1_000_000)
	frac = strings.TrimRight(frac, "0")
	if frac == "" {
		return strconv.FormatUint(whole, 10)
	}
	return strconv.FormatUint(whole, 10) + "." + frac
}
