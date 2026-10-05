package solanatools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
)

func simulationTx(t *testing.T) *solana.Transaction {
	t.Helper()
	key := PrivateKeyFromSeed("bundle-test-key")
	tx, err := solana.NewTransaction([]solana.Instruction{system.NewTransferInstruction(1, key.PublicKey(), solana.SystemProgramID).Build()}, solana.Hash{1}, solana.TransactionPayer(key.PublicKey()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sign(tx, key); err != nil {
		t.Fatal(err)
	}
	return tx
}

func TestSimulateBundleRequest(t *testing.T) {
	tx := simulationTx(t)
	original, _ := EncodeTransaction(tx)
	client, err := NewHelius(HeliusConfig{APIKey: "test-key", RPCURL: "https://example.test/rpc?custom=1", HTTPClient: httpFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Host != "example.test" || req.URL.Query().Get("api-key") != "test-key" || req.URL.Query().Get("custom") != "1" {
			t.Fatal("incorrect endpoint", req.URL)
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Fatal("wrong content type")
		}
		var body struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Method != "simulateBundle" || len(body.Params) != 2 {
			t.Fatal("incorrect RPC method/params")
		}
		var transactions struct {
			EncodedTransactions []string `json:"encodedTransactions"`
		}
		json.Unmarshal(body.Params[0], &transactions)
		if len(transactions.EncodedTransactions) != 2 || transactions.EncodedTransactions[0] != original {
			t.Fatal("incorrect transaction encoding")
		}
		decoded, err := base64.StdEncoding.DecodeString(transactions.EncodedTransactions[1])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := solana.TransactionFromBytes(decoded); err != nil {
			t.Fatal(err)
		}
		var config struct {
			BundleSimulationOptions
			TransactionEncoding string `json:"transactionEncoding"`
		}
		if err := json.Unmarshal(body.Params[1], &config); err != nil {
			t.Fatal(err)
		}
		if config.TransactionEncoding != "base64" || !config.SkipSigVerify || !config.ReplaceRecentBlockhash || len(config.PreExecutionAccountsConfigs) != 2 || len(config.PostExecutionAccountsConfigs) != 2 || config.PreExecutionAccountsConfigs[0] != nil {
			t.Fatal("incorrect config", config)
		}
		if config.PostExecutionAccountsConfigs[1].Addresses[0] != solana.SystemProgramID {
			t.Fatal("lost snapshot address")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{"context":{"slot":42},"value":{"summary":"succeeded","transactionResults":[{"err":null,"logs":["ok"],"unitsConsumed":123},{"err":null,"unitsConsumed":456}]}}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SimulateBundle(context.Background(), []*solana.Transaction{tx, tx}, BundleSimulationOptions{SkipSigVerify: true, ReplaceRecentBlockhash: true, PostExecutionAccountsConfigs: []*BundleAccountsConfig{nil, {Addresses: []solana.PublicKey{solana.SystemProgramID}, Encoding: solana.EncodingBase64}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TransactionResults) != 2 || *result.TransactionResults[0].UnitsConsumed != 123 {
		t.Fatal(result)
	}
	after, _ := EncodeTransaction(tx)
	if after != original {
		t.Fatal("simulation mutated transaction")
	}
}

func TestSimulateBundleResponses(t *testing.T) {
	tx := simulationTx(t)
	cases := []struct {
		name, body                        string
		httpStatus                        int
		wantSimulationError, wantRPCError bool
	}{
		{"failed", `{"jsonrpc":"2.0","id":1,"result":{"summary":{"failed":{"error":"BundleExecutionTimeout","tx_signature":null}},"transactionResults":[{"err":{"InstructionError":[0,"InvalidArgument"]},"logs":["failed"]}]}}`, 200, true, false},
		{"rpc error", `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"Method not found"}}`, 200, false, true},
		{"null result", `{"jsonrpc":"2.0","id":1,"result":null}`, 200, false, false},
		{"missing summary", `{"jsonrpc":"2.0","id":1,"result":{}}`, 200, false, false},
		{"incomplete success", `{"jsonrpc":"2.0","id":1,"result":{"summary":"succeeded","transactionResults":[]}}`, 200, false, false},
		{"invalid json", `broken`, 200, false, false},
		{"rate limit", `{}`, 429, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := NewHelius(HeliusConfig{APIKey: "test-key", HTTPClient: httpFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.httpStatus, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})})
			result, err := h.SimulateBundle(context.Background(), []*solana.Transaction{tx}, BundleSimulationOptions{})
			if err == nil {
				t.Fatal("failure accepted")
			}
			var simErr *BundleSimulationError
			var rpcErr *BundleRPCError
			if errors.As(err, &simErr) != tc.wantSimulationError || errors.As(err, &rpcErr) != tc.wantRPCError {
				t.Fatal("wrong error type", err)
			}
			if tc.wantSimulationError && (result == nil || len(result.TransactionResults[0].Logs) == 0) {
				t.Fatal("lost failure logs")
			}
		})
	}
}

func TestSimulateBundleValidationAndCancellation(t *testing.T) {
	calls := 0
	h, _ := NewHelius(HeliusConfig{APIKey: "test-key", HTTPClient: httpFunc(func(req *http.Request) (*http.Response, error) { calls++; return nil, errors.New(req.URL.String()) })})
	tx := simulationTx(t)
	for _, tc := range []struct {
		txs  []*solana.Transaction
		opts BundleSimulationOptions
	}{
		{nil, BundleSimulationOptions{}},
		{make([]*solana.Transaction, 6), BundleSimulationOptions{}},
		{[]*solana.Transaction{nil}, BundleSimulationOptions{}},
		{[]*solana.Transaction{tx}, BundleSimulationOptions{ReplaceRecentBlockhash: true}},
		{[]*solana.Transaction{tx}, BundleSimulationOptions{PreExecutionAccountsConfigs: []*BundleAccountsConfig{}}},
	} {
		if _, err := h.SimulateBundle(context.Background(), tc.txs, tc.opts); err == nil {
			t.Fatal("invalid bundle accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid requests reached transport")
	}
	_, err := h.SimulateBundle(context.Background(), []*solana.Transaction{tx}, BundleSimulationOptions{})
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatal("credential leak", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = h.SimulateBundle(ctx, []*solana.Transaction{tx}, BundleSimulationOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("context cancellation lost", err)
	}
}
