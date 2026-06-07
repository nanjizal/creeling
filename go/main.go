package main

import (
	"fmt"
)

func main() {
	program := []Node{
		{Kind: NodeVarDecl, VarID: 101},
		{Kind: NodeCall, VarID: 101, MethodID: "varAccessStub"},
		{
			Kind: NodeIfElse, VarID: 0,
			ThenBlock: []Node{
				{Kind: NodeCall, VarID: 101, MethodID: "calculateLength"},
			},
			ElseBlock: []Node{
				{Kind: NodeFieldStore, VarID: 101, Field: "leakedProperty"},
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
