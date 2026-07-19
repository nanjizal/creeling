package main

import (
	"strconv"
)

// Context manages the transient, scoped dictionary map of live variable flow trackers.
// Fields:
//
//	Variables - Active variable symbols indexed by their unique ID
type Context struct {
	Variables map[int]Flow
}

func (ctx *Context) Clone() *Context {
	cloned := &Context{
		Variables: make(map[int]Flow),
	}
	for k, v := range ctx.Variables {
		cloned.Variables[k] = v
	}
	return cloned
}

type Typer struct {
	Instructions []string
	Ctx          *Context
	Tags         []Tag
}

func NewTyper() *Typer {
	return &Typer{Instructions: make([]string, 0)}
}

// ProcessNode analyzes flow semantics and maps variable state changes.
func (t *Typer) ProcessNode(node Node, ctx *Context) {
	switch node.Kind {
	case ECall:
		flow, exists := ctx.Variables[node.VarID]
		argState := Borrow

		if exists && flow.State == Owned {
			argState = Owned
			flow.State = Borrow
			ctx.Variables[node.VarID] = flow
		}

		stateStr := "Borrow"
		if argState == Owned {
			stateStr = "Owned"
		}
		t.Instructions = append(t.Instructions, "CALL_METHOD_Variant_"+stateStr)

	case EField:
		if flow, exists := ctx.Variables[node.VarID]; exists {
			flow.State = Leaked
			ctx.Variables[node.VarID] = flow
			t.Instructions = append(t.Instructions, "UPGRADE_TO_RUNTIME_RC_var"+strconv.Itoa(node.VarID))
		}

	case EBlock:
		for _, childNode := range node.Nodes {
			if childNode.VarID != 0 {
				if _, active := ctx.Variables[childNode.VarID]; !active {
					ctx.Variables[childNode.VarID] = Flow{
						ID:     childNode.VarID,
						State:  Owned,
						Offset: childNode.Offset,
					}
					t.Instructions = append(t.Instructions, "ALLOC_VAR var_"+strconv.Itoa(childNode.VarID))
				}
			}
			t.ProcessNode(childNode, ctx)
		}

	case EIf:
		thenContext := ctx.Clone()
		elseContext := ctx.Clone()
		for _, childNode := range node.ThenBlock {
			t.ProcessNode(childNode, thenContext)
		}
		for _, childNode := range node.ElseBlock {
			t.ProcessNode(childNode, elseContext)
		}
		ctx.Variables = t.MergeBranchUnification(thenContext, elseContext).Variables
	}
}

func (t *Typer) MergeBranchUnification(thenCtx *Context, elseCtx *Context) *Context {
	merged := &Context{
		Variables: make(map[int]Flow),
	}
	allIDs := make(map[int]bool)
	for id := range thenCtx.Variables {
		allIDs[id] = true
	}
	for id := range elseCtx.Variables {
		allIDs[id] = true
	}

	for id := range allIDs {
		thenFlow, inThen := thenCtx.Variables[id]
		elseFlow, inElse := elseCtx.Variables[id]

		if inThen && inElse && thenFlow.State == Owned && elseFlow.State == Owned {
			merged.Variables[id] = Flow{
				ID:     id,
				State:  Owned,
				Offset: thenFlow.Offset,
			}
		} else {
			refOffset := 0
			if inThen {
				refOffset = thenFlow.Offset
			}
			merged.Variables[id] = Flow{
				ID:     id,
				State:  Leaked,
				Offset: refOffset,
			}
		}
	}
	return merged
}

func (t *Typer) ProcessBlock(stream []Node, ctx *Context) {
	for idx, node := range stream {
		t.ProcessNode(node, ctx)

		targetVarID := node.VarID
		if node.Kind == EIf {
			targetVarID = t.findVariableInBranch(node)
		}

		if targetVarID != 0 {
			isUsedLater := t.evaluateLivenessPruning(idx, targetVarID, stream)

			if !isUsedLater {
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

func (t *Typer) evaluateLivenessPruning(currentIdx int, varID int, stream []Node) bool {
	for i := currentIdx + 1; i < len(stream); i++ {
		if t.nodeUsesVariable(stream[i], varID) {
			return true
		}
	}
	return false
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
func (t *Typer) SpecializeAndCheck(program []Node) []string {
	ctx := &Context{Variables: make(map[int]Flow)}
	t.ProcessBlock(program, ctx)
	t.Ctx = ctx

	// Automatically extract and format everything right here
	t.Tags = make([]Tag, 0, len(ctx.Variables))
	for id, f := range ctx.Variables {
		t.Tags = append(t.Tags, Tag{
			Pos:   f.Offset,
			VarID: id,
			Place: byte(f.State),
		})
	}
	return t.Instructions
}
