package solanatools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBalanceDoesNotCache(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"context":{"slot":1},"value":%d}}`, calls)
	}))
	defer server.Close()
	client := New(rpc.New(server.URL))
	for i := uint64(1); i <= 2; i++ {
		balance, err := client.Balance(context.Background(), solana.SystemProgramID)
		if err != nil || balance != i {
			t.Fatal(balance, err)
		}
	}
	if calls != 2 {
		t.Fatal("balance was cached")
	}
}

func TestMissingATAAndLargeBalance(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     any               `json:"id"`
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				var value any
				switch req.Method {
				case "getAccountInfo":
					var address string
					json.Unmarshal(req.Params[0], &address)
					if missing && address != solana.WrappedSol.String() {
						value = nil
					} else {
						value = map[string]any{"owner": solana.TokenProgramID.String(), "lamports": 1, "executable": false, "rentEpoch": 0, "data": []string{"", "base64"}}
					}
				case "getTokenSupply":
					value = map[string]any{"amount": "1000000000", "decimals": 9, "uiAmountString": "1"}
				case "getTokenAccountBalance":
					value = map[string]any{"amount": "18446744073709551615", "decimals": 9, "uiAmountString": "18446744073.709551615"}
				default:
					t.Errorf("unexpected method %s", req.Method)
				}
				json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"context": map[string]any{"slot": 1}, "value": value}})
			}))
			defer server.Close()
			client := New(rpc.New(server.URL))
			balance, err := client.TokenBalance(context.Background(), TokenBalanceParams{Wallet: solana.SystemProgramID, Mint: solana.WrappedSol, Program: solana.TokenProgramID})
			if err != nil {
				t.Fatal(err)
			}
			expected := ^uint64(0)
			if missing {
				expected = 0
			}
			if balance.RawAmount != expected || balance.Decimals != 9 {
				t.Fatal(balance)
			}
		})
	}
}
