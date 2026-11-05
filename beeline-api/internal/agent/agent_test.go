package agent

import (
	"context"
	"strings"
	"testing"
)

type fakeLLM struct {
	out    string
	system string
}

func (f *fakeLLM) Complete(_ context.Context, _, system, _ string) (string, error) {
	f.system = system
	return f.out, nil
}

func TestDecideParsesPayAndReply(t *testing.T) {
	llm := &fakeLLM{out: "Sure!\n```json\n{\"action\":\"pay\",\"recipient\":\"Acme.Goni-Beeline.testnet\",\"amount_usdc\":\"$0.5\",\"memo\":\"invoice 42\",\"thought\":\"pay acme\"}\n```"}
	a := &Agent{LLM: llm, Model: "m", Contacts: []Contact{{"Acme Design", "acme.goni-beeline.testnet"}}}
	d, err := a.Decide(context.Background(), "pay acme")
	if err != nil {
		t.Fatal(err)
	}
	if d.Action != "pay" || d.Recipient != "acme.goni-beeline.testnet" || d.AmountUSDC != "0.5" {
		t.Fatalf("%+v", d)
	}
	if !strings.Contains(llm.system, "acme.goni-beeline.testnet") {
		t.Fatal("contacts must be in the system prompt")
	}
	llm.out = `{"action":"reply","reply":"Hi!","thought":"greeting"}`
	d, err = a.Decide(context.Background(), "hello")
	if err != nil || d.Action != "reply" || d.Reply != "Hi!" {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestDecideRejectsUnusableOutput(t *testing.T) {
	for _, out := range []string{"no json here", `{"action":"pay","recipient":""}`, `{"action":"steal"}`, `{bad json}`} {
		a := &Agent{LLM: &fakeLLM{out: out}}
		if _, err := a.Decide(context.Background(), "x"); err == nil {
			t.Errorf("expected an error for %q", out)
		}
	}
}
