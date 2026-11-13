// Package payin lets a customer pay a real-money invoice from any wallet or exchange. It asks NEAR Intents
// (1Click) for a one-time deposit address that delivers the invoice's exact USDC amount to the vendor on NEAR,
// shows the customer what to send, and tracks the payment. Docs: https://docs.near-intents.org/
//
// How "paid" stays honest (docs/PAYMENT_FLOW.md): an invoice becomes paid only when TWO independent signals
// agree: NEAR Intents reports the swap SUCCESS (provider-reported), and the vendor's own USDC balance inside
// NEAR Intents (mt_balance_of on intents.near, read from the chain) rose by at least the invoice amount since the
// quote was made. The money is delivered to that Intents balance, so the vendor's account needs no USDC registration
// and no NEAR for gas. A provider that lies, or a
// balance that rose for another reason, is not enough on its own. Known limit: an unrelated incoming transfer
// to the vendor can satisfy the balance signal, but the provider signal must still be SUCCESS for this
// invoice's own deposit address.
//
// NEAR Intents has no testnet, so this path is real money and mainnet only. Nothing here moves funds: a quote
// only creates an address, and the customer's own wallet or exchange does the sending.
package payin

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

// Quote and payment states.
const (
	StatusWaiting    = "waiting"
	StatusProcessing = "processing"
	StatusConfirming = "confirming"
	StatusPaid       = "paid"
	StatusIncomplete = "incomplete"
	StatusRefunded   = "refunded"
	StatusFailed     = "failed"
	StatusExpired    = "expired"
)

func terminal(s string) bool {
	switch s {
	case StatusPaid, StatusIncomplete, StatusRefunded, StatusFailed, StatusExpired:
		return true
	}
	return false
}

// InputError is a problem with what the customer asked for (shown to them as is).
type InputError struct{ Message string }

func (e *InputError) Error() string { return e.Message }

func bad(msg string) error { return &InputError{Message: msg} }

// TokenSource lists the assets NEAR Intents supports (*intents.Client does).
type TokenSource interface {
	Tokens(ctx context.Context) ([]intents.Token, error)
}

// BalanceReader reads an account's balance of an asset inside NEAR Intents (base units).
type BalanceReader interface {
	IntentsBalance(ctx context.Context, account, assetID string) (*big.Int, error)
}

// chainNames are the origin chains offered, in display order. NEAR itself is left out: paying from NEAR has its own path.
var chainOrder = []string{"base", "eth", "arb", "op", "pol", "bsc", "avax", "sol", "tron", "ton"}
var chainNames = map[string]string{
	"base": "Base", "eth": "Ethereum", "arb": "Arbitrum", "op": "Optimism", "pol": "Polygon",
	"bsc": "BNB Chain", "avax": "Avalanche", "sol": "Solana", "tron": "Tron", "ton": "TON",
}
var allowedSymbols = map[string]int{"USDC": 0, "USDT": 1} // stablecoins only: the amount to send does not swing with a price

// Option is one thing a customer can pay with.
type Option struct {
	AssetID   string `json:"asset_id"`
	Chain     string `json:"chain"`
	ChainName string `json:"chain_name"`
	Symbol    string `json:"symbol"`
	Decimals  int    `json:"decimals"`
}

var (
	evmRe  = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	solRe  = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)
	tronRe = regexp.MustCompile(`^T[1-9A-HJ-NP-Za-km-z]{33}$`)
	tonRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{48}$`)
)

// ValidateRefund checks the customer's refund address has the right shape for the chain. It cannot know the
// customer owns it: the page makes them say so.
func ValidateRefund(chain, addr string) error {
	ok := false
	switch chain {
	case "eth", "base", "arb", "op", "pol", "bsc", "avax":
		ok = evmRe.MatchString(addr)
	case "sol":
		ok = solRe.MatchString(addr)
	case "tron":
		ok = tronRe.MatchString(addr)
	case "ton":
		ok = tonRe.MatchString(addr)
	}
	if !ok {
		return bad("That is not a valid " + chainNames[chain] + " address. Paste your own address on that network.")
	}
	return nil
}

// Quote is one deposit address for one invoice, and where it stands.
type Quote struct {
	ID            int64     `json:"id"`
	InvoiceID     string    `json:"invoice_id"`
	OriginAsset   string    `json:"origin_asset"`
	Chain         string    `json:"chain"`
	ChainName     string    `json:"chain_name"`
	Symbol        string    `json:"symbol"`
	Decimals      int       `json:"-"`
	AmountInUnits string    `json:"amount_in_units"`
	AmountIn      string    `json:"amount_in"` // what to send, in whole units, e.g. "2.004"
	DepositAddr   string    `json:"deposit_address"`
	DepositMemo   string    `json:"deposit_memo"`
	RefundTo      string    `json:"refund_to"`
	Deadline      time.Time `json:"deadline"`
	Status        string    `json:"status"`
	ProviderState string    `json:"provider_status"` // NEAR Intents' own word, shown as provider-reported
	DeliveredTx   string    `json:"delivered_tx"`
	Message       string    `json:"message"`
	CreatedAt     time.Time `json:"created_at"`

	balanceBefore string
}

// Service creates and tracks pay-in quotes.
type Service struct {
	DB       *pgxpool.Pool
	Intents  intents.Provider
	Tokens   TokenSource
	Balance  BalanceReader
	Invoices *invoices.Service
	Now      func() time.Time
	TTL      time.Duration // how long a deposit address is valid (default 30 minutes)

	mu       sync.Mutex
	cached   []Option
	cachedAt time.Time
}

// Options lists what a customer can pay with: stablecoins on the supported chains that NEAR Intents lists.
func (s *Service) Options(ctx context.Context) ([]Option, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && s.Now().Sub(s.cachedAt) < 10*time.Minute {
		return s.cached, nil
	}
	toks, err := s.Tokens.Tokens(ctx)
	if err != nil {
		if s.cached != nil {
			return s.cached, nil // a stale list beats no list
		}
		return nil, err
	}
	var out []Option
	for _, t := range toks {
		name, okChain := chainNames[t.Blockchain]
		_, okSym := allowedSymbols[t.Symbol]
		if !okChain || !okSym || t.Decimals <= 0 || t.Decimals > 24 || t.AssetID == "" {
			continue
		}
		out = append(out, Option{AssetID: t.AssetID, Chain: t.Blockchain, ChainName: name, Symbol: t.Symbol, Decimals: t.Decimals})
	}
	rank := map[string]int{}
	for i, c := range chainOrder {
		rank[c] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Chain != out[j].Chain {
			return rank[out[i].Chain] < rank[out[j].Chain]
		}
		return allowedSymbols[out[i].Symbol] < allowedSymbols[out[j].Symbol]
	})
	s.cached, s.cachedAt = out, s.Now()
	return out, nil
}

func human(units string, decimals int) string {
	n, ok := new(big.Int).SetString(units, 10)
	if !ok {
		return units
	}
	d := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole, frac := new(big.Int).DivMod(n, d, new(big.Int))
	if frac.Sign() == 0 {
		return whole.String()
	}
	f := fmt.Sprintf("%0*s", decimals, frac.String())
	return whole.String() + "." + strings.TrimRight(f, "0")
}

func (q *Quote) finish() *Quote {
	q.AmountIn = human(q.AmountInUnits, q.Decimals)
	q.Message = messageFor(q)
	return q
}

func messageFor(q *Quote) string {
	switch q.Status {
	case StatusWaiting:
		return fmt.Sprintf("Waiting for your deposit. Send exactly %s %s on %s to the address above.", q.AmountIn, q.Symbol, q.ChainName)
	case StatusProcessing:
		return "We can see your deposit. NEAR Intents is converting and delivering it to the vendor."
	case StatusConfirming:
		return "Delivered by NEAR Intents. Confirming it on NEAR…"
	case StatusPaid:
		return "Paid."
	case StatusIncomplete:
		return "You sent less than required. NEAR Intents refunds it to your refund address. Start again to pay in full."
	case StatusRefunded:
		return "This payment was refunded to your address. Nothing reached the vendor."
	case StatusFailed:
		return "This payment did not go through. If you sent funds, they are refunded to your refund address."
	case StatusExpired:
		return "This deposit address expired. Do not send anything to it. Start again for a new one."
	}
	return ""
}

const quoteCols = `id, invoice_id, origin_asset, origin_chain, origin_symbol, origin_decimals, amount_in, deposit_address, deposit_memo,
	refund_to, deadline, status, provider_status, delivered_tx, balance_before, created_at`

func scan(row pgx.Row) (*Quote, error) {
	var q Quote
	err := row.Scan(&q.ID, &q.InvoiceID, &q.OriginAsset, &q.Chain, &q.Symbol, &q.Decimals, &q.AmountInUnits, &q.DepositAddr, &q.DepositMemo,
		&q.RefundTo, &q.Deadline, &q.Status, &q.ProviderState, &q.DeliveredTx, &q.balanceBefore, &q.CreatedAt)
	if err != nil {
		return nil, err
	}
	q.ChainName = chainNames[q.Chain]
	return q.finish(), nil
}

// Current returns the newest quote for an invoice, or nil.
func (s *Service) Current(ctx context.Context, invoiceID string) (*Quote, error) {
	q, err := scan(s.DB.QueryRow(ctx, `SELECT `+quoteCols+` FROM payin_quotes WHERE invoice_id = $1 ORDER BY id DESC LIMIT 1`, invoiceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return q, err
}

func token(inv *invoices.Invoice) string { return strings.TrimPrefix(inv.Asset, "nep141:") }

// Create asks NEAR Intents for a deposit address that delivers exactly the invoice amount to the vendor.
// Asking again for the same invoice, asset and refund address returns the quote that is still live.
func (s *Service) Create(ctx context.Context, inv *invoices.Invoice, originAsset, refundTo string) (*Quote, error) {
	if inv.Asset != invoices.MainnetUSDC {
		return nil, bad("Paying from other chains works for real-money invoices only. This is a test invoice.")
	}
	if inv.Status != "open" {
		return nil, bad("This invoice is already paid.")
	}
	refundTo = strings.TrimSpace(refundTo)
	opts, err := s.Options(ctx)
	if err != nil {
		return nil, fmt.Errorf("token list: %w", err)
	}
	var opt *Option
	for i := range opts {
		if opts[i].AssetID == originAsset {
			opt = &opts[i]
		}
	}
	if opt == nil {
		return nil, bad("That token is not supported. Pick one from the list.")
	}
	if err := ValidateRefund(opt.Chain, refundTo); err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	if q, err := scan(s.DB.QueryRow(ctx, `SELECT `+quoteCols+` FROM payin_quotes WHERE invoice_id = $1 AND origin_asset = $2 AND refund_to = $3
		AND status = 'waiting' AND deadline > $4 ORDER BY id DESC LIMIT 1`, inv.ID, originAsset, refundTo, now.Add(3*time.Minute))); err == nil {
		return q, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	ttl := s.TTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	deadline := now.Add(ttl)
	if cap := inv.ExpiresAt.Add(-time.Minute); deadline.After(cap) {
		deadline = cap
	}
	if deadline.Before(now.Add(5 * time.Minute)) {
		return nil, bad("This invoice expires too soon to pay from another chain. Ask the vendor for a new one.")
	}

	// Baseline for the delivery check: what the vendor holds right now.
	before, err := s.Balance.IntentsBalance(ctx, inv.Payout, inv.Asset)
	if err != nil {
		return nil, fmt.Errorf("read vendor balance: %w", err)
	}
	resp, err := s.Intents.Quote(ctx, intents.QuoteRequest{
		Dry: false, SwapType: "EXACT_OUTPUT", SlippageTolerance: 100,
		OriginAsset: opt.AssetID, DepositType: "ORIGIN_CHAIN", DestinationAsset: inv.Asset,
		Amount: inv.Amount, RefundTo: refundTo, RefundType: "ORIGIN_CHAIN",
		Recipient: inv.Payout, RecipientType: "INTENTS", // lands in the vendor's NEAR Intents balance: no token registration, no gas
		Deadline: deadline.Format("2006-01-02T15:04:05.000Z"),
	})
	if err != nil {
		return nil, fmt.Errorf("quote: %w", err)
	}
	qt := resp.Quote
	amtIn, ok1 := new(big.Int).SetString(qt.AmountIn, 10)
	amtOut, ok2 := new(big.Int).SetString(qt.AmountOut, 10)
	want, _ := new(big.Int).SetString(inv.Amount, 10)
	if !ok1 || amtIn.Sign() <= 0 || !ok2 || amtOut.Cmp(want) < 0 {
		return nil, errors.New("quote did not deliver the invoice amount")
	}
	if a := qt.DepositAddress; a == "" || len(a) > 200 || strings.ContainsAny(a, " \t\r\n") || len(qt.DepositMemo) > 120 {
		return nil, errors.New("quote carried no usable deposit address")
	}
	if qt.Deadline != nil && qt.Deadline.Before(deadline) {
		deadline = qt.Deadline.UTC()
	}
	var id int64
	err = s.DB.QueryRow(ctx, `INSERT INTO payin_quotes (invoice_id, origin_asset, origin_chain, origin_symbol, origin_decimals, amount_in, deposit_address, deposit_memo,
		refund_to, deadline, balance_before, quote_json) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		inv.ID, opt.AssetID, opt.Chain, opt.Symbol, opt.Decimals, qt.AmountIn, qt.DepositAddress, qt.DepositMemo, refundTo, deadline, before.String(), []byte(resp.Raw)).Scan(&id)
	if err != nil {
		return nil, err
	}
	return scan(s.DB.QueryRow(ctx, `SELECT `+quoteCols+` FROM payin_quotes WHERE id = $1`, id))
}

// Refresh reads the quote's status from NEAR Intents and the vendor's balance from NEAR, advances the quote,
// and marks the invoice paid only when both agree. It never moves money.
func (s *Service) Refresh(ctx context.Context, inv *invoices.Invoice) (*Quote, error) {
	q, err := s.Current(ctx, inv.ID)
	if err != nil || q == nil || terminal(q.Status) {
		return q, err
	}
	now := s.Now().UTC()
	next, tx := q.Status, q.DeliveredTx
	provider := q.ProviderState
	st, serr := s.Intents.Status(ctx, q.DepositAddr, q.DepositMemo)
	switch {
	case serr == nil:
		provider = st.Status
		if len(st.SwapDetails.DestinationChainTxHashes) > 0 {
			tx = st.SwapDetails.DestinationChainTxHashes[0].Hash
		}
		switch st.Status {
		case intents.StatusPendingDeposit:
			next = StatusWaiting
		case intents.StatusKnownDepositTx, intents.StatusProcessing:
			next = StatusProcessing
		case intents.StatusSuccess:
			next = StatusConfirming
		case intents.StatusIncompleteDeposit:
			next = StatusIncomplete
		case intents.StatusRefunded:
			next = StatusRefunded
		case intents.StatusFailed:
			next = StatusFailed
		}
	default:
		var ie *intents.Error
		if !(errors.As(serr, &ie) && ie.Kind == intents.KindNotFound) {
			return q, nil // cannot reach NEAR Intents right now: keep what we have, nothing is lost
		}
		next = StatusWaiting // no deposit seen yet
	}
	if next == StatusWaiting && now.After(q.Deadline) {
		next = StatusExpired
	}

	balanceAfter := ""
	if next == StatusConfirming {
		after, err := s.Balance.IntentsBalance(ctx, inv.Payout, inv.Asset)
		if err == nil {
			balanceAfter = after.String()
			before, _ := new(big.Int).SetString(q.balanceBefore, 10)
			want, _ := new(big.Int).SetString(inv.Amount, 10)
			if before != nil && want != nil && new(big.Int).Sub(after, before).Cmp(want) >= 0 {
				record := func(hash string) error {
					_, err := s.Invoices.RecordPayment(context.WithoutCancel(ctx), inv.ID, invoices.Payment{
						TxHash: hash, Sender: "cross-chain:" + q.Chain, Receiver: inv.Payout, Token: token(inv), Amount: inv.Amount,
						Memo: "intents:" + q.DepositAddr, BlockHash: "intents", BlockTime: now, Verification: "intents-delivered",
					})
					return err
				}
				hash := tx
				if hash == "" {
					hash = "intents:" + q.DepositAddr
				}
				perr := record(hash)
				if errors.Is(perr, invoices.ErrTxReused) && hash != "intents:"+q.DepositAddr {
					// The provider's hash already pays something else: fall back to the one-time deposit address, which is unique.
					perr = record("intents:" + q.DepositAddr)
				}
				switch {
				case perr == nil:
					next = StatusPaid
				case errors.Is(perr, invoices.ErrAlreadyPaid), errors.Is(perr, invoices.ErrTxReused):
					// Only call it paid if this invoice really holds a payment; otherwise leave it for the next check.
					if cur, _ := s.Invoices.PaymentFor(ctx, inv.ID); cur != nil {
						_ = s.Invoices.AddEvent(ctx, inv.ID, "ALREADY_PAID", "A second payment arrived by another chain after this invoice was already paid. The vendor received it as well.", hash)
						next = StatusPaid
					}
				default:
					return q, perr
				}
			}
		}
	}

	if next != q.Status || provider != q.ProviderState || tx != q.DeliveredTx || balanceAfter != "" {
		_, err := s.DB.Exec(ctx, `UPDATE payin_quotes SET status = $2, provider_status = $3, delivered_tx = $4,
			balance_after = CASE WHEN $5 = '' THEN balance_after ELSE $5 END, updated_at = now() WHERE id = $1 AND status = $6`,
			q.ID, next, provider, tx, balanceAfter, q.Status)
		if err != nil {
			return q, err
		}
		if next != q.Status {
			s.noteTransition(ctx, inv.ID, next, tx)
		}
	}
	return s.Current(ctx, inv.ID)
}

// noteTransition records, on the vendor's invoice, the outcomes that are not a success.
func (s *Service) noteTransition(ctx context.Context, invoiceID, status, tx string) {
	kind, reason := "", ""
	switch status {
	case StatusRefunded:
		kind, reason = "PAYIN_REFUNDED", "A payment from another chain was refunded to the customer. Nothing reached you."
	case StatusFailed:
		kind, reason = "PAYIN_FAILED", "A payment from another chain failed and is refunded to the customer."
	case StatusIncomplete:
		kind, reason = "PAYIN_INCOMPLETE", "A customer sent less than the required amount from another chain. It is refunded to them."
	}
	if kind != "" {
		_ = s.Invoices.AddEvent(ctx, invoiceID, kind, reason, tx)
	}
}
