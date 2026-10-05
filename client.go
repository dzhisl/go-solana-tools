package solanatools

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// ErrAccountNotFound indicates that an account does not exist on chain.
var ErrAccountNotFound = errors.New("account not found")

// Option configures a Client at construction time.
type Option func(*Client)

// Client is immutable after construction and never caches or starts workers.
type Client struct {
	rpc              *rpc.Client
	tokenAccountsRPC *rpc.Client
	commitment       rpc.CommitmentType
}

// New creates a read-only client with confirmed commitment by default.
// The application supplies the RPC client and owns its configuration.
func New(client *rpc.Client, options ...Option) *Client {
	c := &Client{rpc: client, tokenAccountsRPC: client, commitment: rpc.CommitmentConfirmed}
	for _, option := range options {
		if option != nil {
			option(c)
		}
	}
	return c
}

// WithCommitment sets the commitment used for RPC reads.
func WithCommitment(commitment rpc.CommitmentType) Option {
	return func(c *Client) { c.commitment = commitment }
}

// WithTokenAccountsRPC routes owner scans to a separately configured RPC endpoint.
func WithTokenAccountsRPC(client *rpc.Client) Option {
	return func(c *Client) { c.tokenAccountsRPC = client }
}

func (c *Client) validate() error {
	if c == nil || c.rpc == nil {
		return fmt.Errorf("RPC client is required")
	}
	return nil
}

// Balance fetches the wallet's SOL balance in integer lamports.
func (c *Client) Balance(ctx context.Context, wallet solana.PublicKey) (uint64, error) {
	if err := c.validate(); err != nil {
		return 0, err
	}
	res, err := c.rpc.GetBalance(ctx, wallet, c.commitment)
	if err != nil {
		return 0, fmt.Errorf("get balance: %w", err)
	}
	if res == nil {
		return 0, fmt.Errorf("empty balance response")
	}
	return res.Value, nil
}

// TokenBalanceParams identifies a wallet's ATA by wallet, mint and token program.
type TokenBalanceParams struct {
	Wallet  solana.PublicKey
	Mint    solana.PublicKey
	Program solana.PublicKey
}

// TokenBalance reads the associated account only, not all accounts of the owner.
// An absent ATA returns zero with decimals obtained from the mint.
func (c *Client) TokenBalance(ctx context.Context, params TokenBalanceParams) (TokenAmount, error) {
	ata, err := AssociatedTokenAddress(params.Wallet, params.Mint, params.Program)
	if err != nil {
		return TokenAmount{}, err
	}
	balance, err := c.TokenAccountBalance(ctx, ata)
	if !errors.Is(err, ErrAccountNotFound) {
		return balance, err
	}
	mint, err := c.Mint(ctx, params.Mint)
	if err != nil {
		return TokenAmount{}, err
	}
	if mint.Program != params.Program {
		return TokenAmount{}, fmt.Errorf("mint token program does not match requested program")
	}
	return NewTokenAmount(0, mint.Supply.Decimals), nil
}

// TokenAccountBalance reads an explicit token account; absence returns ErrAccountNotFound.
func (c *Client) TokenAccountBalance(ctx context.Context, account solana.PublicKey) (TokenAmount, error) {
	if err := c.validate(); err != nil {
		return TokenAmount{}, err
	}
	// Check existence explicitly; do not infer absence from RPC error strings.
	info, err := c.rpc.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{Commitment: c.commitment})
	if errors.Is(err, rpc.ErrNotFound) {
		return TokenAmount{}, ErrAccountNotFound
	}
	if err != nil {
		return TokenAmount{}, fmt.Errorf("get token account: %w", err)
	}
	if info == nil || info.Value == nil {
		return TokenAmount{}, ErrAccountNotFound
	}
	if err := validateTokenProgram(info.Value.Owner); err != nil {
		return TokenAmount{}, err
	}
	res, err := c.rpc.GetTokenAccountBalance(ctx, account, c.commitment)
	if err != nil {
		return TokenAmount{}, fmt.Errorf("get token balance: %w", err)
	}
	if res == nil || res.Value == nil {
		return TokenAmount{}, fmt.Errorf("empty token balance response")
	}
	raw, err := strconv.ParseUint(res.Value.Amount, 10, 64)
	if err != nil {
		return TokenAmount{}, fmt.Errorf("invalid raw token amount: %w", err)
	}
	return NewTokenAmount(raw, res.Value.Decimals), nil
}

// AccountProgram returns the on-chain account's owning program, not a token wallet owner.
func (c *Client) AccountProgram(ctx context.Context, account solana.PublicKey) (solana.PublicKey, error) {
	if err := c.validate(); err != nil {
		return solana.PublicKey{}, err
	}
	res, err := c.rpc.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{Commitment: c.commitment})
	if errors.Is(err, rpc.ErrNotFound) {
		return solana.PublicKey{}, ErrAccountNotFound
	}
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("get account program: %w", err)
	}
	if res == nil || res.Value == nil {
		return solana.PublicKey{}, ErrAccountNotFound
	}
	return res.Value.Owner, nil
}

// MintInfo describes a mint's owning token program, supply and decimals.
type MintInfo struct {
	Address solana.PublicKey
	Program solana.PublicKey
	Supply  TokenAmount
}

// Mint fetches the token program, total raw supply and decimals of a mint.
func (c *Client) Mint(ctx context.Context, address solana.PublicKey) (MintInfo, error) {
	program, err := c.AccountProgram(ctx, address)
	if err != nil {
		return MintInfo{}, err
	}
	if err := validateTokenProgram(program); err != nil {
		return MintInfo{}, err
	}
	res, err := c.rpc.GetTokenSupply(ctx, address, c.commitment)
	if err != nil {
		return MintInfo{}, fmt.Errorf("get token supply: %w", err)
	}
	if res == nil || res.Value == nil {
		return MintInfo{}, fmt.Errorf("empty token supply response")
	}
	raw, err := strconv.ParseUint(res.Value.Amount, 10, 64)
	if err != nil {
		return MintInfo{}, fmt.Errorf("invalid token supply: %w", err)
	}
	return MintInfo{Address: address, Program: program, Supply: NewTokenAmount(raw, res.Value.Decimals)}, nil
}

// LatestBlockhash preserves lastValidBlockHeight for the application's expiry handling.
func (c *Client) LatestBlockhash(ctx context.Context) (*rpc.GetLatestBlockhashResult, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c.rpc.GetLatestBlockhash(ctx, c.commitment)
}

// LargestTokenAccounts returns token account addresses, not aggregated wallet holders.
func (c *Client) LargestTokenAccounts(ctx context.Context, mint solana.PublicKey) (*rpc.GetTokenLargestAccountsResult, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c.rpc.GetTokenLargestAccounts(ctx, mint, c.commitment)
}

// Transaction fetches a legacy or v0 transaction, including metadata, by signature.
// The RPC requires confirmed or finalized commitment for this method.
func (c *Client) Transaction(ctx context.Context, signature solana.Signature) (*rpc.GetTransactionResult, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	version := uint64(0)
	return c.rpc.GetTransaction(ctx, signature, &rpc.GetTransactionOpts{Commitment: c.commitment, MaxSupportedTransactionVersion: &version})
}
