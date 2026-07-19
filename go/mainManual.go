//go:build manual
// +build manual

package main

import (
	"fmt"
)

func main() {
	// Replicates your exact AST loop structure using the newly unified layout
	program := []Node{
		{Kind: ELocal, VarID: 101, Offset: 12}, // Declared & Owned at stream byte index 12
		{Kind: ECall, VarID: 101, Offset: 16},  // Borrowed momentarily for evaluation
		{
			Kind:   EIf,
			VarID:  0,
			Offset: 20,
			ThenBlock: []Node{
				{Kind: ECall, VarID: 101, Offset: 24}, // Branch A borrows resource safely
			},
			ElseBlock: []Node{
				{Kind: EField, VarID: 101, Offset: 28}, // Branch B leaks reference to runtime heap
			},
		},
	}

	typerInstance := NewTyper()
	compiledInstructions := typerInstance.SpecializeAndCheck(program)

	fmt.Println("=== Linearity & Data-Flow Diagnostic Trace ===")
	for _, inst := range compiledInstructions {
		fmt.Printf(" [Bytecode Instruction] -> %s\n", inst)
	}

	fmt.Println("\n=== Final Context Variable States ===")
	if typerInstance.Ctx != nil {
		for id, fRecord := range typerInstance.Ctx.Variables {
			stateName := "UNKNOWN"
			switch fRecord.State {
			case Owned:
				stateName = "OWNED"
			case Borrow:
				stateName = "BORROW"
			case Leaked:
				stateName = "LEAKED"
			case Free:
				stateName = "FREE (LFR3)"
			}
			fmt.Printf(" VarID %d -> Status: %s (Registered at Node Offset: %d)\n",
				id, stateName, fRecord.Offset)
		}
	}
}
