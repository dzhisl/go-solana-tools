package solanatools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// BundleAccountsConfig selects account snapshots for one transaction in the bundle.
type BundleAccountsConfig struct {
	Addresses []solana.PublicKey  `json:"addresses"`
	Encoding  solana.EncodingType `json:"encoding,omitempty"`
}

// BundleSimulationOptions configures simulation; zero values keep signature checks
// and original blockhashes. Snapshot slices must have one entry per transaction;
// nil slices become arrays of null entries. A nil entry disables that snapshot.
type BundleSimulationOptions struct {
	SkipSigVerify                bool                    `json:"skipSigVerify"`
	ReplaceRecentBlockhash       bool                    `json:"replaceRecentBlockhash"`
	PreExecutionAccountsConfigs  []*BundleAccountsConfig `json:"preExecutionAccountsConfigs"`
	PostExecutionAccountsConfigs []*BundleAccountsConfig `json:"postExecutionAccountsConfigs"`
}

// BundleTransactionResult contains execution diagnostics for one transaction.
// Account entries may be nil when accounts do not exist; Err is raw Solana error JSON.
type BundleTransactionResult struct {
	Err                   json.RawMessage `json:"err"`
	Logs                  []string        `json:"logs"`
	PreExecutionAccounts  []*rpc.Account  `json:"preExecutionAccounts"`
	PostExecutionAccounts []*rpc.Account  `json:"postExecutionAccounts"`
	UnitsConsumed         *uint64         `json:"unitsConsumed"`
	ReturnData            json.RawMessage `json:"returnData"`
}

// BundleSimulationResult preserves the bundle summary and per-transaction diagnostics.
// Summary is "succeeded" or {"failed":{"error":...,"tx_signature":...}}.
type BundleSimulationResult struct {
	Summary            json.RawMessage           `json:"summary"`
	TransactionResults []BundleTransactionResult `json:"transactionResults"`
}

// BundleSimulationError reports failed execution while the returned result retains logs.
type BundleSimulationError struct {
	Summary json.RawMessage
}

// Error describes a failed bundle simulation, including the server's failure summary.
func (e *BundleSimulationError) Error() string {
	return fmt.Sprintf("bundle simulation failed: %s", e.Summary)
}

// BundleRPCError reports an RPC-level failure such as an unsupported method.
type BundleRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error describes the RPC failure code and message.
func (e *BundleRPCError) Error() string {
	return fmt.Sprintf("simulateBundle RPC error %d: %s", e.Code, e.Message)
}

// SimulateBundle executes 1–5 transactions in order against Helius's Jito-enabled
// RPC without submitting them. Transactions are encoded as base64 and not mutated.
// Execution failure returns both diagnostics and *BundleSimulationError; transport,
// RPC and malformed-response errors return a nil result. The endpoint must support
// simulateBundle. Caller owns context deadlines and rate limiting.
func (h *HeliusClient) SimulateBundle(ctx context.Context, transactions []*solana.Transaction, options BundleSimulationOptions) (*BundleSimulationResult, error) {
	if h == nil || h.http == nil || h.rpcURL == "" {
		return nil, fmt.Errorf("Helius client is required")
	}
	count := len(transactions)
	if count < 1 || count > 5 {
		return nil, fmt.Errorf("Jito bundle must contain 1–5 transactions")
	}
	if options.ReplaceRecentBlockhash && !options.SkipSigVerify {
		return nil, fmt.Errorf("replacing blockhash requires SkipSigVerify")
	}
	if options.PreExecutionAccountsConfigs == nil {
		options.PreExecutionAccountsConfigs = make([]*BundleAccountsConfig, count)
	}
	if options.PostExecutionAccountsConfigs == nil {
		options.PostExecutionAccountsConfigs = make([]*BundleAccountsConfig, count)
	}
	if len(options.PreExecutionAccountsConfigs) != count || len(options.PostExecutionAccountsConfigs) != count {
		return nil, fmt.Errorf("account snapshot config lengths must match bundle length")
	}
	encoded, err := EncodeTransactions(transactions)
	if err != nil {
		return nil, err
	}
	// The transactions object and the simulation config are distinct RPC parameters.
	config := struct {
		BundleSimulationOptions
		TransactionEncoding string `json:"transactionEncoding"`
	}{options, "base64"}
	request := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}{"2.0", 1, "simulateBundle", []any{struct {
		EncodedTransactions []string `json:"encodedTransactions"`
	}{encoded}, config}}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode bundle request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create bundle request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("bundle simulation request failed") // Transport URLs include credentials.
	}
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("empty bundle simulation response")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Helius simulation returned HTTP %d", resp.StatusCode)
	}
	const maxBody = 16 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read bundle response: %w", err)
	}
	if len(data) > maxBody {
		return nil, fmt.Errorf("bundle response exceeds limit")
	}
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *BundleRPCError `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode bundle response: %w", err)
	}
	if envelope.JSONRPC != "2.0" || envelope.ID != 1 {
		return nil, fmt.Errorf("invalid bundle RPC response envelope")
	}
	if envelope.Error != nil {
		return nil, envelope.Error
	}
	if len(envelope.Result) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Result), []byte("null")) {
		return nil, fmt.Errorf("missing bundle simulation result")
	}
	// Jito-enabled providers may wrap results in context/value.
	var wrapped struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(envelope.Result, &wrapped); err != nil {
		return nil, fmt.Errorf("decode bundle result: %w", err)
	}
	resultData := envelope.Result
	if len(wrapped.Value) != 0 {
		resultData = wrapped.Value
	}
	var result BundleSimulationResult
	if err := json.Unmarshal(resultData, &result); err != nil {
		return nil, fmt.Errorf("decode bundle diagnostics: %w", err)
	}
	var status string
	if json.Unmarshal(result.Summary, &status) == nil && status == "succeeded" {
		if len(result.TransactionResults) != count {
			return nil, fmt.Errorf("successful simulation returned incorrect transaction count")
		}
		for _, tx := range result.TransactionResults {
			if len(tx.Err) > 0 && !bytes.Equal(bytes.TrimSpace(tx.Err), []byte("null")) {
				return &result, &BundleSimulationError{Summary: tx.Err}
			}
		}
		return &result, nil
	}
	var failure struct {
		Failed json.RawMessage `json:"failed"`
	}
	if json.Unmarshal(result.Summary, &failure) == nil && len(failure.Failed) > 0 && !bytes.Equal(bytes.TrimSpace(failure.Failed), []byte("null")) {
		return &result, &BundleSimulationError{Summary: result.Summary}
	}
	return nil, fmt.Errorf("unrecognized bundle simulation summary")
}
