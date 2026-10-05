package solanatools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gagliardetto/solana-go"
)

// ErrFundingNotFound indicates that Helius has no funding source for the wallet.
var ErrFundingNotFound = errors.New("funding source not found")

// HTTPDoer is an application-supplied HTTP transport, including timeout/retry policy.
type HTTPDoer interface {
	// Do executes the request using the application's HTTP transport policy.
	Do(*http.Request) (*http.Response, error)
}

// HeliusConfig supplies credentials, endpoints and HTTP transport for Helius queries.
type HeliusConfig struct {
	APIKey string
	// BaseURL defaults to https://api.helius.xyz. HTTPClient is supplied by the application.
	BaseURL string
	// RPCURL defaults to https://mainnet.helius-rpc.com; APIKey is added to its query.
	RPCURL     string
	HTTPClient HTTPDoer
}

// HeliusClient provides funding queries and bundle simulation with no internal cache.
type HeliusClient struct {
	apiKey  string
	baseURL string
	rpcURL  string
	http    HTTPDoer
}

// NewHelius creates a Helius client with an explicit API key and HTTP transport.
func NewHelius(config HeliusConfig) (*HeliusClient, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("Helius API key is required")
	}
	if config.HTTPClient == nil {
		return nil, fmt.Errorf("HTTP client is required")
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://api.helius.xyz"
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return nil, fmt.Errorf("invalid Helius base URL")
	}
	if config.RPCURL == "" {
		config.RPCURL = "https://mainnet.helius-rpc.com"
	}
	rpcURL, err := url.Parse(config.RPCURL)
	if err != nil || rpcURL.Host == "" || (rpcURL.Scheme != "https" && rpcURL.Scheme != "http") || rpcURL.User != nil || rpcURL.Fragment != "" {
		return nil, fmt.Errorf("invalid Helius RPC URL")
	}
	query := rpcURL.Query()
	query.Set("api-key", config.APIKey)
	rpcURL.RawQuery = query.Encode()
	return &HeliusClient{apiKey: config.APIKey, baseURL: strings.TrimRight(config.BaseURL, "/"), rpcURL: rpcURL.String(), http: config.HTTPClient}, nil
}

// FundingInfo describes the funding transaction reported by the Helius Wallet API.
type FundingInfo struct {
	Funder      string      `json:"funder"`
	FunderName  *string     `json:"funderName"`
	FunderType  *string     `json:"funderType"`
	Mint        string      `json:"mint"`
	Symbol      string      `json:"symbol"`
	Amount      json.Number `json:"amount"`
	AmountRaw   string      `json:"amountRaw"`
	Decimals    int         `json:"decimals"`
	Date        string      `json:"date"`
	Signature   string      `json:"signature"`
	Timestamp   int64       `json:"timestamp"`
	Slot        uint64      `json:"slot"`
	ExplorerURL string      `json:"explorerUrl"`
}

// FundingSource fetches wallet funding metadata; absence returns ErrFundingNotFound.
func (h *HeliusClient) FundingSource(ctx context.Context, wallet solana.PublicKey) (*FundingInfo, error) {
	if h == nil || h.http == nil {
		return nil, fmt.Errorf("Helius client is required")
	}
	endpoint := h.baseURL + "/v1/wallet/" + wallet.String() + "/funded-by?" + url.Values{"api-key": []string{h.apiKey}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create funding request")
	}
	resp, err := h.http.Do(req)
	// Do not propagate transport URL errors: the URL contains the API key.
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("funding request failed")
	}
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("empty funding response")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrFundingNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Helius returned HTTP %d", resp.StatusCode)
	}
	const maxBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read funding response: %w", err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("funding response exceeds limit")
	}
	var info FundingInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("decode funding response: %w", err)
	}
	return &info, nil
}
