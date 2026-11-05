package playground

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

// Tamper modes simulate what a scammer does with a copied pay link.
const (
	TamperNone       = "none"        // pay the genuine invoice
	TamperSwapPayout = "swap_payout" // replace the vendor's payout address with the attacker's
	TamperEditAmount = "edit_amount" // multiply the amount by 10
)

// PayInvoice has the agent pay a vendor invoice through the vault, optionally after a scammer
// has edited the link. The vault, not this code, decides: the edited payload no longer matches
// the issuer's signature.
func (s *Service) PayInvoice(ctx context.Context, inv *invoices.Invoice, tamper string) (*Result, error) {
	payload, recipient, amount := inv.Payload, inv.Payout, inv.Amount
	switch tamper {
	case TamperNone, "":
	case TamperSwapPayout:
		attacker := strings.ToLower(s.Cfg.Attacker.Address)
		payload = strings.Replace(payload, `"payout":"`+inv.Payout+`"`, `"payout":"`+attacker+`"`, 1)
		recipient = attacker
	case TamperEditAmount:
		a, ok := new(big.Int).SetString(inv.Amount, 10)
		if !ok {
			return nil, errors.New("bad invoice amount")
		}
		amount = a.Mul(a, big.NewInt(10)).String()
		payload = strings.Replace(payload, `"amount":"`+inv.Amount+`"`, `"amount":"`+amount+`"`, 1)
	default:
		return nil, errors.New("tamper must be none, swap_payout or edit_amount")
	}
	if tamper != TamperNone && tamper != "" && payload == inv.Payload {
		return nil, errors.New("could not apply the tampering to this invoice")
	}

	var items []string
	for _, it := range inv.Items {
		items = append(items, fmt.Sprintf("%d x %s", it.Quantity, it.Name))
	}
	task := fmt.Sprintf(
		"Pay this vendor invoice from a pay link. Vendor: %s %s. Order: %s. Items: %s. Pay %s base units of USDC to %s on Base.\nInvoice from the link: %s",
		inv.VendorName, inv.VendorHandle, inv.Memo, strings.Join(items, ", "), amount, recipient, payload)

	return s.Run(ctx, Input{
		Scenario:  ScenarioInvoice,
		Task:      task,
		Recipient: recipient,
		Amount:    amount,
		Invoice:   &InvoiceArg{Payload: payload, Signature: inv.Signature},
	})
}
