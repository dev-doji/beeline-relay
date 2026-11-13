package accounts

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

// This is a command parser, not an AI model: it understands a fixed, documented set of phrasings
// and says so when it does not. Prices are handled as decimal strings and converted to integer
// base units without floating point. It is the fallback when the AI model is unavailable.
//
//	invoice 25 for logo design
//	bill 2 x linen shirt 0.15, delivery 0.05
//	invoice website work 3.5 usdc
//	make me an invoice for 5 inks at 100 naira each   (naira is converted at Switch's rate)

var (
	leadRe    = regexp.MustCompile(`(?i)^\s*(?:\b(?:please|yes|yeah|ok|okay|so|then|now|kindly|abeg|can|could|you|i|want|need|would|like|to)\b\s*)*(?:create|make|send|generate|new)?\s*(?:me|us)?\s*(?:an?\s+|the\s+)?(invoice|bill|payment\s+link|pay\s+link|charge|request)\s*(to|for)?\s*`)
	splitRe   = regexp.MustCompile(`(?i)\s*(?:,|;|\+|\band\b|\n)\s*`)
	qtyRe     = regexp.MustCompile(`(?i)^\s*(\d{1,3})\s*x\s+`)
	qtyWordRe = regexp.MustCompile(`(?i)^\s*(\d{1,3})\s+[a-z]`)
	skipQtyRe = regexp.MustCompile(`(?i)^\s*\d{1,3}\s+(for|at|of|x|usdc|usd|naira|ngn)\b`)
	numRe     = regexp.MustCompile(`\d+(?:\.\d{1,6})?`)
	noiseRe   = regexp.MustCompile(`(?i)\b(usdc|usd|dollars?|at|of|for|each|only|me|naira|ngn)\b|[$₦]`)
	perRe     = regexp.MustCompile(`(?i)\b(per|each)\b.*$`)
	commaRe   = regexp.MustCompile(`(\d),(\d{3})`)
	spaceRe   = regexp.MustCompile(`\s+`)
	helpText  = `I can make an invoice from a message like "invoice 25 for logo design" or "bill 2 x linen shirt 0.15, delivery 0.05". Amounts are in test USDC. Say "naira" to price in naira, for example "invoice 5 inks at 100 naira each".`
)

// ErrNotInvoice means the message is not an invoice request.
var ErrNotInvoice = errors.New(helpText)

// ParseInvoice turns a chat message into invoice items priced in USDC.
func ParseInvoice(msg string) ([]invoices.Item, error) { return parseInvoice(msg, "") }

// ParseInvoiceNGN is ParseInvoice for a message priced in naira; rate is naira per 1 USDC. Prices are
// converted to USDC base units, rounding up, and each line keeps its naira price in its name.
func ParseInvoiceNGN(msg, rate string) ([]invoices.Item, error) { return parseInvoice(msg, rate) }

func parseInvoice(msg, rate string) ([]invoices.Item, error) {
	msg = strings.TrimSpace(msg)
	if msg == "" || len(msg) > 500 {
		return nil, errors.New("Please send a short message, for example: " + helpText)
	}
	if rate != "" {
		for commaRe.MatchString(msg) {
			msg = commaRe.ReplaceAllString(msg, "$1$2")
		}
	}
	loc := leadRe.FindStringIndex(msg)
	if loc == nil {
		return nil, ErrNotInvoice
	}
	rest := msg[loc[1]:]
	var items []invoices.Item
	for _, seg := range splitRe.Split(rest, -1) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		qty := 1
		if m := qtyRe.FindStringSubmatch(seg); m != nil {
			qty, _ = strconv.Atoi(m[1])
			seg = seg[len(m[0]):]
		} else if m := qtyWordRe.FindStringSubmatch(seg); m != nil && !skipQtyRe.MatchString(seg) && len(numRe.FindAllString(seg, -1)) >= 2 {
			// "5 inks at 100 naira": the first number is the quantity, the last is the price.
			qty, _ = strconv.Atoi(m[1])
			seg = strings.TrimSpace(seg[len(m[1]):])
		}
		nums := numRe.FindAllString(seg, -1)
		if len(nums) == 0 {
			return nil, errors.New(`I could not find a price in "` + seg + `". ` + helpText)
		}
		price := nums[len(nums)-1]
		var units string
		ok := false
		if rate != "" {
			if u, err := ngnToUnits(price, rate); err == nil {
				units, ok = u, true
			}
		} else {
			units, ok = usdcUnits(price)
		}
		if !ok {
			return nil, errors.New(`"` + price + `" is not a valid amount. Use a positive amount like 25 or 0.15.`)
		}
		name := seg[:strings.LastIndex(seg, price)] + seg[strings.LastIndex(seg, price)+len(price):]
		name = perRe.ReplaceAllString(name, "")
		name = strings.TrimSpace(spaceRe.ReplaceAllString(noiseRe.ReplaceAllString(name, " "), " "))
		if name == "" {
			name = "Payment"
		}
		if rate != "" {
			name = clip(name, 50) + " (₦" + niceNaira(price) + " each)"
		}
		if len(name) > 80 {
			name = name[:80]
		}
		items = append(items, invoices.Item{Name: name, Quantity: qty, UnitPrice: units})
	}
	if len(items) == 0 {
		return nil, ErrNotInvoice
	}
	return items, nil
}

// usdcUnits converts "0.15" to "150000"; false if zero or malformed.
func usdcUnits(s string) (string, bool) {
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || len(whole) > 6 || len(frac) > 6 {
		return "", false
	}
	u := strings.TrimLeft(whole+(frac + "000000")[:6], "0")
	return u, u != ""
}
