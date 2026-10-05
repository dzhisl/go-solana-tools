# ADR 0001: Single public package with application-owned dependencies

Status: accepted from user requirements.

All exported helpers live in the root package `solanatools`, module `github.com/dzhisl/go-solana-tools`. Domain separation uses source files, avoiding separate imports. The dependency is the existing gagliardetto SDK; its public types are reused.

RPC operations are methods on `Client`, initialized with an application-provided RPC client. Pure operations are functions. Commitment defaults to confirmed and is explicitly configurable. There are no global clients, configuration readers, caches, background workers, storage integration or logging dependencies introduced by this module.

Submission, confirmation and leader scheduling are excluded. Following the user's additional request, Jito bundle simulation is available through HeliusClient with explicit options and diagnostics. Amounts use unsigned raw units and exact decimal strings. Missing accounts and missing funding sources have explicit sentinel errors. Provider tip addresses remain application configuration, rather than hardcoded mutable provider lists. The custom mixer program builder is omitted until its program contract and configuration are specified.

Funding queries use a separate optional Helius client in the same root package. The application supplies the HTTP transport, credentials and rate limiting. Bundler migration and GitHub publication are separate operations.
