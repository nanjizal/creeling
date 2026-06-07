package main

import (
	"strconv"
)

type State int

const (
	Owned State = iota
	Borrow
	Leaked
)

type NodeType int

const (
	NodeCall NodeType = iota
	NodeFieldStore
	NodeVarDecl
	NodeIfElse
	NodeBlock
)

type Node struct {
	Kind      NodeType
	VarID     int
	MethodID  string
	Field     string
	Nodes     []Node
	ThenBlock []Node
	ElseBlock []Node
	//Args      []Node
}

type VariableTrack struct {
	ID    int
	State State
}

type Context struct {
	Variables map[int]VariableTrack
}

// CreelingTyper
type Typer struct {
	Instructions []string
}

func NewTyper() *Typer {
	return &Typer{Instructions: make([]string, 0)}
}

// Flow analysis
func (t *Typer) ProcessNode(node Node, ctx *Context) {
	switch node.Kind {
	case NodeCall:
		track, exists := ctx.Variables[node.VarID]
		argState := Borrow

		if exists && track.State == Owned {
			argState = Owned
			track.State = Borrow
			ctx.Variables[node.VarID] = track
		}

		stateStr := "Borrow"
		if argState == Owned {
			stateStr = "Owned"
		}
		t.Instructions = append(t.Instructions, "CALL_METHOD"+"_Variant_"+stateStr)

	case NodeFieldStore:
		if track, exists := ctx.Variables[node.VarID]; exists {
			track.State = Leaked
			ctx.Variables[node.VarID] = track
			t.Instructions = append(t.Instructions, "UPGRADE_TO_RUNTIME_RC_var"+strconv.Itoa(node.VarID))
		}

	case NodeVarDecl:
		ctx.Variables[node.VarID] = VariableTrack{
			ID:    node.VarID,
			State: Owned,
		}
		t.Instructions = append(t.Instructions, "ALLOC_VAR var_"+strconv.Itoa(node.VarID))
	case NodeIfElse:
		thenContext := ctx.Clone()
		elseContext := ctx.Clone()
		for _, childNode := range node.ThenBlock {
			t.ProcessNode(childNode, thenContext)
		}
		for _, childNode := range node.ElseBlock {
			t.ProcessNode(childNode, elseContext)
		}
		ctx.Variables = t.MergeBranchUnification(thenContext, elseContext).Variables
	case NodeBlock:
		for _, childNode := range node.Nodes {
			t.ProcessNode(childNode, ctx)
		}
	}
}

func (ctx *Context) Clone() *Context {
	cloned := &Context{
		Variables: make(map[int]VariableTrack),
	}
	for k, v := range ctx.Variables {
		cloned.Variables[k] = v
	}
	return cloned
}

func (t *Typer) ProcessBlock(stream []Node, ctx *Context) {
	for idx, node := range stream {
		t.ProcessNode(node, ctx)

		targetVarID := node.VarID
		if node.Kind == NodeIfElse {
			targetVarID = t.findVariableInBranch(node)
		}

		if targetVarID != 0 {
			// FIX 1: Pass targetVarID here instead of node.VarID!
			isUsedLater := t.evaluateLivenessPruning(idx, targetVarID, stream)

			if !isUsedLater {
				// FIX 2: Check and remove using targetVarID as well
				if _, exists := ctx.Variables[targetVarID]; exists {
					t.Instructions = append(t.Instructions, "LFR3_FREE var_"+strconv.Itoa(targetVarID))
					delete(ctx.Variables, targetVarID)
				}
			}
		}
	}
}

func (t *Typer) findVariableInBranch(node Node) int {
	for _, child := range node.ThenBlock {
		if child.VarID != 0 {
			return child.VarID
		}
	}
	for _, child := range node.ElseBlock {
		if child.VarID != 0 {
			return child.VarID
		}
	}
	return 0
}
func (t *Typer) MergeBranchUnification(thenCtx *Context, elseCtx *Context) *Context {
	merged := &Context{
		Variables: make(map[int]VariableTrack),
	}
	allIDs := make(map[int]bool)
	for id := range thenCtx.Variables {
		allIDs[id] = true
	}
	for id := range elseCtx.Variables {
		allIDs[id] = true
	}

	for id := range allIDs {
		thenTrack, inThen := thenCtx.Variables[id]
		elseTrack, inElse := elseCtx.Variables[id]
		if inThen && inElse && thenTrack.State == Owned && elseTrack.State == Owned {
			merged.Variables[id] = VariableTrack{
				ID:    id,
				State: Owned,
			}
		} else {
			merged.Variables[id] = VariableTrack{
				ID:    id,
				State: Leaked,
			}
		}
	}
	return merged
}
func (t *Typer) evaluateLivenessPruning(currentIdx int, varID int, stream []Node) bool {
	for i := currentIdx + 1; i < len(stream); i++ {
		if t.nodeUsesVariable(stream[i], varID) {
			return true
		}
	}
	return false
}
func (t *Typer) SpecializeAndCheck(program []Node) []string {
	ctx := &Context{
		Variables: make(map[int]VariableTrack),
	}
	t.ProcessBlock(program, ctx)
	return t.Instructions
}
func (t *Typer) nodeUsesVariable(node Node, varID int) bool {
	if node.VarID == varID {
		return true
	}
	for _, child := range node.ThenBlock {
		if t.nodeUsesVariable(child, varID) {
			return true
		}
	}
	for _, child := range node.ElseBlock {
		if t.nodeUsesVariable(child, varID) {
			return true
		}
	}
	return false
}
