package solanatools

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/programs/system"
)

func validateTokenProgram(program solana.PublicKey) error {
	if program != solana.TokenProgramID && program != solana.Token2022ProgramID {
		return fmt.Errorf("unsupported token program %s", program)
	}
	return nil
}

// AssociatedTokenAddress derives an ATA for legacy SPL Token or Token-2022.
func AssociatedTokenAddress(wallet, mint, program solana.PublicKey) (solana.PublicKey, error) {
	if err := validateTokenProgram(program); err != nil {
		return solana.PublicKey{}, err
	}
	address, _, err := solana.FindProgramAddress([][]byte{wallet.Bytes(), program.Bytes(), mint.Bytes()}, solana.SPLAssociatedTokenAccountProgramID)
	return address, err
}

// NewCreateATAIdempotentInstruction builds ATA creation that tolerates an existing ATA.
func NewCreateATAIdempotentInstruction(payer, wallet, mint, program solana.PublicKey) (solana.Instruction, error) {
	ata, err := AssociatedTokenAddress(wallet, mint, program)
	if err != nil {
		return nil, err
	}
	return solana.NewInstruction(solana.SPLAssociatedTokenAccountProgramID, []*solana.AccountMeta{
		solana.Meta(payer).WRITE().SIGNER(), solana.Meta(ata).WRITE(), solana.Meta(wallet), solana.Meta(mint), solana.Meta(solana.SystemProgramID), solana.Meta(program),
	}, []byte{1}), nil
}

// TransferParams specifies an ATA-to-ATA transfer in raw units with checked decimals.
type TransferParams struct {
	Mint, FromOwner, ToOwner, TokenProgram solana.PublicKey
	Amount                                 uint64
	Decimals                               uint8
}

// NewTransferCheckedInstruction transfers between ATAs. It does not create the destination.
// Token-2022 extensions needing additional accounts (e.g. transfer hooks) are not handled.
func NewTransferCheckedInstruction(p TransferParams) (solana.Instruction, error) {
	from, err := AssociatedTokenAddress(p.FromOwner, p.Mint, p.TokenProgram)
	if err != nil {
		return nil, err
	}
	to, err := AssociatedTokenAddress(p.ToOwner, p.Mint, p.TokenProgram)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 10)
	data[0] = 12
	binary.LittleEndian.PutUint64(data[1:9], p.Amount)
	data[9] = p.Decimals
	return solana.NewInstruction(p.TokenProgram, []*solana.AccountMeta{solana.Meta(from).WRITE(), solana.Meta(p.Mint), solana.Meta(to).WRITE(), solana.Meta(p.FromOwner).SIGNER()}, data), nil
}

// NewCloseTokenAccountInstruction builds account closure, sending reclaimed SOL to destination.
// Authority is a single signer; closure conditions are enforced by the token program.
func NewCloseTokenAccountInstruction(account, destination, authority, program solana.PublicKey) (solana.Instruction, error) {
	if err := validateTokenProgram(program); err != nil {
		return nil, err
	}
	return solana.NewInstruction(program, []*solana.AccountMeta{solana.Meta(account).WRITE(), solana.Meta(destination).WRITE(), solana.Meta(authority).SIGNER()}, []byte{9}), nil
}

// NewBurnCheckedInstruction builds a token burn with mint decimals validation.
func NewBurnCheckedInstruction(account, mint, authority, program solana.PublicKey, amount uint64, decimals uint8) (solana.Instruction, error) {
	if err := validateTokenProgram(program); err != nil {
		return nil, err
	}
	data := make([]byte, 10)
	data[0] = 15
	binary.LittleEndian.PutUint64(data[1:9], amount)
	data[9] = decimals
	return solana.NewInstruction(program, []*solana.AccountMeta{solana.Meta(account).WRITE(), solana.Meta(mint).WRITE(), solana.Meta(authority).SIGNER()}, data), nil
}

// NewSyncNativeInstruction synchronizes a legacy wrapped-SOL account's token balance.
func NewSyncNativeInstruction(account solana.PublicKey) solana.Instruction {
	return solana.NewInstruction(solana.TokenProgramID, []*solana.AccountMeta{solana.Meta(account).WRITE()}, []byte{17})
}

// NewWrapSOLInstructions creates the wallet's WSOL ATA, transfers payer's lamports and syncs it.
func NewWrapSOLInstructions(payer, wallet solana.PublicKey, amount uint64) ([]solana.Instruction, error) {
	ata, err := AssociatedTokenAddress(wallet, solana.WrappedSol, solana.TokenProgramID)
	if err != nil {
		return nil, err
	}
	create, err := NewCreateATAIdempotentInstruction(payer, wallet, solana.WrappedSol, solana.TokenProgramID)
	if err != nil {
		return nil, err
	}
	return []solana.Instruction{create, system.NewTransferInstruction(amount, payer, ata).Build(), NewSyncNativeInstruction(ata)}, nil
}

// NewPriorityFeeInstruction computes micro-lamports per CU from a total fee budget.
// The price rounds down; compute-unit limit must be set separately by the caller.
func NewPriorityFeeInstruction(lamports, computeUnits uint64) (solana.Instruction, error) {
	if computeUnits == 0 {
		return nil, fmt.Errorf("compute units must be positive")
	}
	price := new(big.Int).SetUint64(lamports)
	price.Mul(price, big.NewInt(1_000_000))
	price.Div(price, new(big.Int).SetUint64(computeUnits))
	if !price.IsUint64() {
		return nil, fmt.Errorf("compute unit price overflow")
	}
	return computebudget.NewSetComputeUnitPriceInstruction(price.Uint64()).Build(), nil
}

// NewTipInstruction leaves provider destination selection to the application.
func NewTipInstruction(payer, destination solana.PublicKey, lamports uint64) solana.Instruction {
	return system.NewTransferInstruction(lamports, payer, destination).Build()
}

// NewCreateAccountWithSeedInstruction builds seeded account creation with explicit rent and space.
// Payer funds the account; base signs the derivation; owner is the destination program.
func NewCreateAccountWithSeedInstruction(payer, base, owner solana.PublicKey, seed string, lamports, space uint64) (solana.Instruction, error) {
	if len(seed) > 32 {
		return nil, fmt.Errorf("seed exceeds 32 bytes")
	}
	account, err := solana.CreateWithSeed(base, seed, owner)
	if err != nil {
		return nil, err
	}
	return system.NewCreateAccountWithSeedInstruction(base, seed, lamports, space, owner, payer, account, base).Build(), nil
}
