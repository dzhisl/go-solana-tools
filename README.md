# go-solana-tools

Reusable Solana helpers with one public import:

```go
import tools "github.com/dzhisl/go-solana-tools"
```

The Go package name is `solanatools`; the alias `tools` is optional. Requires Go 1.26.4. Uses `gagliardetto/solana-go` types, so existing SDK clients, keys and instructions work directly.

## Initialization and USDT balance

```go
import (
    "context"
    "fmt"

    tools "github.com/dzhisl/go-solana-tools"
    "github.com/gagliardetto/solana-go"
    "github.com/gagliardetto/solana-go/rpc"
)

func printUSDT(ctx context.Context, endpoint, walletAddress, usdtMintAddress string) error {
    wallet, err := solana.PublicKeyFromBase58(walletAddress)
    if err != nil { return err }
    mint, err := solana.PublicKeyFromBase58(usdtMintAddress)
    if err != nil { return err }

    utils := tools.New(rpc.New(endpoint), tools.WithCommitment(rpc.CommitmentConfirmed))
    balance, err := utils.TokenBalance(ctx, tools.TokenBalanceParams{
        Wallet: wallet,
        Mint: mint,
        Program: solana.TokenProgramID,
    })
    if err != nil { return err }
    fmt.Println(balance.UIAmount) // exact string, e.g. "12.5"
    return nil
}
```

`TokenBalance` reads the wallet's ATA, not an aggregate of all token accounts. For an absent ATA it returns zero with mint decimals. `TokenAccountBalance` accepts a token account directly and returns `ErrAccountNotFound` if absent. Errors from RPC are preserved. Direct token-account reads use an existence lookup plus a balance query; these are separate chain snapshots.

## Available helpers

- Exact amounts: `ParseSOL`, `FormatSOL`, `ParseAmount`, `FormatAmount`.
- Local transaction operations: `Sign`, `PartialSign`, `IsSigner`, `EncodeTransaction(s)`.
- Keys: `PrivateKeyFromSeed` preserves SHA-256 + ed25519 derivation. Use secret, high-entropy seeds.
- Instructions: ATA create, transfer checked, burn checked, close, wrap SOL, sync native, seeded account creation, priority fee and generic tip transfer.
- RPC reads: balances, mint supply/program, transactions, latest blockhash (including expiry height), largest token accounts and ALT.
- Owner scans: `TokenAccounts`, `NonZeroTokenMints`, `TokenAccountCounts` support legacy Token and Token-2022. Use `WithTokenAccountsRPC` for an endpoint with account indexes.
- Optional Helius: `NewHelius(HeliusConfig{APIKey: key, HTTPClient: client})`, then `FundingSource` or `SimulateBundle`.

No caches, workers, transaction sending, confirmation or leader scheduling. Applications own timeouts, rate limiting, retries, caching and provider tip destinations. Helius HTTP transport and credentials are supplied explicitly. Token-2022 helpers cover base instructions; extension-specific accounts and multisig authorities require custom instructions.

## Jito bundle simulation through Helius

```go
helius, err := tools.NewHelius(tools.HeliusConfig{
    APIKey: apiKey,
    HTTPClient: &http.Client{Timeout: 15 * time.Second},
    // RPCURL is optional; defaults to https://mainnet.helius-rpc.com.
})
if err != nil { return err }

result, err := helius.SimulateBundle(ctx, transactions, tools.BundleSimulationOptions{
    SkipSigVerify: true,
    ReplaceRecentBlockhash: true,
})
if result != nil {
    for _, tx := range result.TransactionResults {
        fmt.Println(tx.Logs, tx.UnitsConsumed, string(tx.Err))
    }
}
if err != nil { return err }
```

`transactions` is a slice of 1–5 `*solana.Transaction` values in execution order. The method calls `simulateBundle` once; it does not simulate each transaction independently. The endpoint must support Jito bundle simulation. A zero options value checks signatures and keeps blockhashes. Replacing blockhashes requires skipping signature verification. Account snapshot configuration accepts one entry per transaction; omitted slices become arrays of null entries.

An execution failure returns both the result (including logs) and `*BundleSimulationError`. An RPC failure returns `*BundleRPCError`; missing or unrecognized summaries are errors. Simulation does not submit transactions or guarantee inclusion. Live Helius access depends on the supplied endpoint; unit tests use mock responses.

Protocol references: [Helius's simulateBundle benchmark coverage](https://github.com/helius-labs/rpc-perf-dash/blob/main/docs/methodology.md), [Jito bundle simulation schema](https://docs.triton.one/trading-apis/bundle-simulation-with-jito), [JSON-RPC request example](https://www.quicknode.com/docs/solana/simulateBundle).

## Installation

After publishing the repository and a version tag:

```sh
go get github.com/dzhisl/go-solana-tools@v0.1.0
```

Before publication, a consuming project can use a local replacement:

```sh
go mod edit -replace github.com/dzhisl/go-solana-tools=/absolute/path/go-solana-tools
go get github.com/dzhisl/go-solana-tools
```

This workspace does not publish a GitHub repository automatically.
