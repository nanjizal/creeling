package main

import (
	"fmt"
)

func main() {
	program := []Node{
		{Kind: ELocal, VarID: 101}, // Map NodeVarDecl/NodeLocalRef to Simon's ELocal (2)
		{Kind: ECall, VarID: 101, MethodID: "varAccessStub"},
		{
			Kind:  EIf, // Map NodeIfElse natively to Simon's EIf (17)
			VarID: 0,
			ThenBlock: []Node{
				{Kind: ECall, VarID: 101, MethodID: "calculateLength"},
			},
			ElseBlock: []Node{
				{Kind: EField, VarID: 101, Field: "LeakedProperty"}, // Map NodeFieldStore to EField (5)
			},
		},
	}
	typerInstance := NewTyper()
	compiledInstructions := typerInstance.SpecializeAndCheck(program)
	fmt.Println("Generated Annotated")
	for _, inst := range compiledInstructions {
		fmt.Printf(" _ %s\n", inst)
	}
}
