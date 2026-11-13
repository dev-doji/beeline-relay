package chainverify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	token    = "usdc.fakes.testnet"
	payee    = "ms-clothing.testnet"
	payer    = "customer.testnet"
	invoice  = "inv_abc123"
	txHash   = "9Zk3wYxq7mT2vRfN8bLcA4dEhJ5uPgS6oQnX1iVyBtKa"
	blockH   = "BlockHash1111111111111111111111111111111111"
	amountOK = "2000000"
)

var (
	paidAt    = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	notExpire = paidAt.Add(time.Hour)
)

// fakeRPC is a TEST DOUBLE for a NEAR node. The transaction shape is modelled on a real `tx` response
// (checked 2026-11-13): final_execution_status, status, transaction{signer_id, receiver_id, actions},
// transaction_outcome.block_hash, receipts_outcome[].outcome.status.
type fakeRPC struct {
	tx      map[string]any
	txErr   error
	blockNs string
	calls   []string
}

func (f *fakeRPC) Call(_ context.Context, method string, params any, out any) error {
	f.calls = append(f.calls, method)
	switch method {
	case "tx":
		if f.txErr != nil {
			return f.txErr
		}
		b, _ := json.Marshal(f.tx)
		return json.Unmarshal(b, out)
	case "block":
		b, _ := json.Marshal(map[string]any{"header": map[string]any{"hash": blockH, "timestamp_nanosec": f.blockNs}})
		return json.Unmarshal(b, out)
	}
	return &RPCError{Name: "METHOD_NOT_FOUND"}
}

type txOpts struct {
	signer, receiver, method, to, amount, memo, deposit string
	noMemo                                              bool
	finalStatus                                         string
	status                                              any
	extraActions                                        int
	failReceipt                                         bool
}

func goodOpts() txOpts {
	return txOpts{signer: payer, receiver: token, method: "ft_transfer", to: payee, amount: amountOK, memo: "beeline:" + invoice, deposit: "1", finalStatus: "FINAL"}
}

func build(o txOpts) map[string]any {
	args := map[string]any{"receiver_id": o.to, "amount": o.amount}
	if !o.noMemo {
		args["memo"] = o.memo
	}
	rawArgs, _ := json.Marshal(args)
	action := map[string]any{"FunctionCall": map[string]any{"method_name": o.method, "args": base64.StdEncoding.EncodeToString(rawArgs), "gas": 30000000000000, "deposit": o.deposit}}
	actions := []any{action}
	for i := 0; i < o.extraActions; i++ {
		actions = append(actions, map[string]any{"Transfer": map[string]any{"deposit": "1"}})
	}
	status := o.status
	if status == nil {
		status = map[string]any{"SuccessValue": ""}
	}
	recStatus := map[string]any{"SuccessValue": ""}
	if o.failReceipt {
		recStatus = map[string]any{"Failure": map[string]any{"ActionError": map[string]any{"kind": "FunctionCallError"}}}
	}
	return map[string]any{
		"final_execution_status": o.finalStatus,
		"status":                 status,
		"transaction":            map[string]any{"signer_id": o.signer, "receiver_id": o.receiver, "actions": actions},
		"transaction_outcome":    map[string]any{"block_hash": blockH, "outcome": map[string]any{"status": map[string]any{"SuccessReceiptId": "r1"}}},
		"receipts_outcome": []any{
			map[string]any{"outcome": map[string]any{"executor_id": o.receiver, "status": recStatus}},
			map[string]any{"outcome": map[string]any{"executor_id": o.signer, "status": map[string]any{"SuccessValue": ""}}},
		},
	}
}

func params() Params {
	return Params{TxHash: txHash, Sender: payer, Token: token, Receiver: payee, Amount: amountOK, Memo: "beeline:" + invoice, ExpiresAt: notExpire}
}

func run(t *testing.T, f *fakeRPC) Result {
	t.Helper()
	f.blockNs = itoa(paidAt.UnixNano())
	return (&Verifier{RPC: f}).Verify(context.Background(), params())
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestAnExactDirectTransferIsVerified(t *testing.T) {
	r := run(t, &fakeRPC{tx: build(goodOpts())})
	if r.Status != StatusVerified || r.Evidence == nil {
		t.Fatalf("%+v", r)
	}
	e := r.Evidence
	if e.Sender != payer || e.Receiver != payee || e.Token != token || e.Amount != amountOK || e.Memo != "beeline:"+invoice || !e.BlockTime.Equal(paidAt) || e.BlockHash != blockH {
		t.Fatalf("evidence: %+v", e)
	}
}

func TestEveryMismatchIsRejectedWithItsOwnReason(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*txOpts)
		code string
	}{
		{"look-alike token contract", func(o *txOpts) { o.receiver = "usdc.fakes.testnet.evil.near" }, CodeWrongToken},
		{"wrong payee", func(o *txOpts) { o.to = "scammer.testnet" }, CodeWrongReceiver},
		{"underpaid", func(o *txOpts) { o.amount = "1999999" }, CodeWrongAmount},
		{"overpaid", func(o *txOpts) { o.amount = "2000001" }, CodeWrongAmount},
		{"memo of another invoice", func(o *txOpts) { o.memo = "beeline:inv_other" }, CodeWrongMemo},
		{"no memo", func(o *txOpts) { o.noMemo = true }, CodeWrongMemo},
		{"no 1 yocto deposit", func(o *txOpts) { o.deposit = "0" }, CodeWrongDeposit},
		{"not ft_transfer", func(o *txOpts) { o.method = "ft_transfer_call" }, CodeUnsupportedCall},
		{"extra actions batched in", func(o *txOpts) { o.extraActions = 1 }, CodeUnsupportedCall},
		{"signed by someone else", func(o *txOpts) { o.signer = "someone-else.testnet" }, CodeWrongSender},
		{"failed on chain (top level)", func(o *txOpts) { o.status = map[string]any{"Failure": map[string]any{}} }, CodeFailed},
		{"a receipt failed", func(o *txOpts) { o.failReceipt = true }, CodeFailed},
	}
	for _, c := range cases {
		o := goodOpts()
		c.mod(&o)
		r := run(t, &fakeRPC{tx: build(o)})
		if r.Status != StatusRejected || r.Code != c.code || r.Reason == "" || r.Evidence != nil {
			t.Errorf("%s: want rejected %s, got %+v", c.name, c.code, r)
		}
	}
}

func TestNotFinalIsPendingNotRejected(t *testing.T) {
	o := goodOpts()
	o.finalStatus = "EXECUTED_OPTIMISTIC"
	if r := run(t, &fakeRPC{tx: build(o)}); r.Status != StatusPending {
		t.Fatalf("%+v", r)
	}
}

func TestNodeProblemsAreHandledWithoutAccusingTheCustomer(t *testing.T) {
	// A timeout means "not seen yet", an unknown hash means "no such transaction", anything else is "try again".
	if r := run(t, &fakeRPC{txErr: &RPCError{Name: "HANDLER_ERROR", Cause: "TIMEOUT_ERROR"}}); r.Status != StatusPending {
		t.Fatalf("timeout: %+v", r)
	}
	if r := run(t, &fakeRPC{txErr: &RPCError{Name: "HANDLER_ERROR", Cause: "UNKNOWN_TRANSACTION"}}); r.Status != StatusRejected || r.Code != CodeNotFound {
		t.Fatalf("unknown: %+v", r)
	}
	if r := run(t, &fakeRPC{txErr: context.DeadlineExceeded}); r.Status != StatusPending {
		t.Fatalf("transport: %+v", r)
	}
}

func TestAPaymentAfterTheInvoiceExpiredIsRejected(t *testing.T) {
	f := &fakeRPC{tx: build(goodOpts())}
	f.blockNs = itoa(notExpire.Add(time.Second).UnixNano())
	r := (&Verifier{RPC: f}).Verify(context.Background(), params())
	if r.Status != StatusRejected || r.Code != CodeExpired {
		t.Fatalf("%+v", r)
	}
	// Exactly at the deadline is still in time.
	f.blockNs = itoa(notExpire.UnixNano())
	if r := (&Verifier{RPC: f}).Verify(context.Background(), params()); r.Status != StatusVerified {
		t.Fatalf("at the deadline: %+v", r)
	}
}

func TestHTTPRPCReadsErrorsAndResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Method string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "tx":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"beeline","error":{"name":"HANDLER_ERROR","cause":{"name":"UNKNOWN_TRANSACTION","info":{"requested_transaction_hash":"x"}}}}`))
		default:
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"beeline","result":{"header":{"hash":"h","timestamp":1794557000000000000,"timestamp_nanosec":"1794557000000000000"}}}`))
		}
	}))
	defer srv.Close()
	// The first endpoint is dead; the second answers.
	c := NewHTTPRPC([]string{"http://127.0.0.1:1", srv.URL}, 3*time.Second)
	var tx txResponse
	err := c.Call(context.Background(), "tx", map[string]string{}, &tx)
	re, ok := err.(*RPCError)
	if !ok || re.Cause != "UNKNOWN_TRANSACTION" {
		t.Fatalf("err = %v", err)
	}
	var blk blockResponse
	if err := c.Call(context.Background(), "block", map[string]string{}, &blk); err != nil {
		t.Fatal(err)
	}
	if blk.Header.Hash != "h" || string(blk.Header.TimestampNano) != `"1794557000000000000"` {
		t.Fatalf("%+v", blk)
	}
}
