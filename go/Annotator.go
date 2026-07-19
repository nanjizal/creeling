package main

import (
	"bytes"
	"encoding/binary"
)

// HxbStructureParser represents your downstream structural target interface contract.
type HxbStructureParser interface{ ReaderApi }

// Annotator uses Anonymous Embedding to "extend" the base Reader class.
type Annotator struct {
	*Reader         // ANONYMOUS EMBEDDING: Annotator inherits ALL methods of Reader!
	TyperObj *Typer // Explicit reference link to your typer.go analyzer
	Buf      []byte // Raw input binary file stream buffer
	Pos      int    // Dynamic byte position tracker managed by the fixed-window scanner
	Tags     []Tag  // The minimal, non-redundant metadata splicing marker collection array
	Nodes    []Node // The local expression node slice cache managed by the composite context
}

// NewAnnotator creates a statically referenced, cross-platform porting-safe constructor.
func NewAnnotator(b []byte, r *Reader, t *Typer) *Annotator {
	return &Annotator{
		Reader:   r,
		TyperObj: t,
		Buf:      b,
	}
}

// --- Checked Primitive Slices (State-Retaining Error Interceptors) ---

func (r *Reader) checkUleb() int {
	if r.err != nil {
		return 0
	}
	val := r.checkVarint()
	return val
}

// readExpression parses an EXD/EFD/AFD bytecode stream into a location-tracked Node AST.
func (r *Reader) readExpression() (Node, error) {
	offset := r.currentExprOffset
	r.currentExprOffset++

	op := FullOpcode(r.checkByte())
	node := Node{Kind: op, Offset: offset}
	if op == Eof || r.err != nil {
		return node, r.err
	}

	switch op {
	case EConst, EType:
		r.checkUleb()
	case ELocal:
		node.VarID = r.checkUleb()
	case EArray, EIn, EBinop:
		if op == EBinop {
			r.checkByte()
		}
		left := r.checkExpr()
		right := r.checkExpr()
		node.Nodes = []Node{left, right}
		if left.VarID != 0 {
			node.VarID = left.VarID
		}
	case EField, ECheckType, EMeta:
		target := r.checkExpr()
		r.checkUleb()
		node.Nodes = []Node{target}
		node.VarID = target.VarID
	case EParenthesis, EUntyped, EThrow:
		inner := r.checkExpr()
		node.Nodes = []Node{inner}
		node.VarID = inner.VarID
	case EObjectDecl:
		count := r.checkUleb()
		for i := 0; i < count; i++ {
			r.checkUleb()
			node.Nodes = append(node.Nodes, r.checkExpr())
		}
	case EArrayDecl, EBlock:
		count := r.checkUleb()
		for i := 0; i < count; i++ {
			node.Nodes = append(node.Nodes, r.checkExpr())
		}
	case ECall, ENew:
		var target Node
		if op == ECall {
			target = r.checkExpr()
			node.VarID = target.VarID
		} else {
			r.checkUleb()
		}
		argCount := r.checkUleb()
		args := make([]Node, 0, argCount)
		for i := 0; i < argCount; i++ {
			args = append(args, r.checkExpr())
		}
		if op == ECall {
			node.Nodes = append([]Node{target}, args...)
		} else {
			node.Nodes = args
		}
	case EUnop:
		r.checkByte()
		r.checkByte()
		target := r.checkExpr()
		node.Nodes = []Node{target}
		node.VarID = target.VarID
	case EFunction:
		r.checkUleb()
	case EFor:
		r.checkUleb()
		iter := r.checkExpr()
		body := r.checkExpr()
		node.Nodes = []Node{iter, body}
	case EIf:
		cond := r.checkExpr()
		thenExpr := r.checkExpr()
		node.Nodes = []Node{cond}
		node.ThenBlock = []Node{thenExpr}
		if r.checkByte() == 1 {
			node.ElseBlock = []Node{r.checkExpr()}
		}
	case EWhile:
		cond := r.checkExpr()
		body := r.checkExpr()
		r.checkByte()
		node.Nodes = []Node{cond, body}
	case EReturn:
		if r.checkByte() == 1 {
			val := r.checkExpr()
			node.Nodes = []Node{val}
			node.VarID = val.VarID
		}
	case ECast:
		target := r.checkExpr()
		node.Nodes = []Node{target}
		node.VarID = target.VarID
		if r.checkByte() == 1 {
			r.checkUleb()
		}
	case ETernary:
		cond := r.checkExpr()
		thenExpr := r.checkExpr()
		elseExpr := r.checkExpr()
		node.Nodes = []Node{cond}
		node.ThenBlock = []Node{thenExpr}
		node.ElseBlock = []Node{elseExpr}
	case EBreak, EContinue:
		return node, r.err
	}
	return node, r.err
}

// Pass1 decodes the primary HXB chunk stream into cached Node trees on the Annotator.
func (a *Annotator) Pass1(targetApi ReaderApi) (*Module, error) {
	a.Pos = 0
	a.Tags = nil
	m, err := a.Reader.Read(targetApi)
	if err != nil {
		return nil, err
	}
	a.Nodes = a.Reader.Nodes
	return m, nil
}

// writeCrLChunk serializes tags into a structured, length-framed binary crL chunk.
func writeCrLChunk(out *bytes.Buffer, tags []Tag) {
	if len(tags) == 0 {
		return
	}
	var crlBuf bytes.Buffer
	writeUleb128(&crlBuf, len(tags))
	for _, t := range tags {
		writeUleb128(&crlBuf, t.Pos)
		writeUleb128(&crlBuf, t.VarID)
		crlBuf.WriteByte(t.Place)
	}
	out.WriteString(string(crL))
	binary.Write(out, binary.BigEndian, int32(crlBuf.Len()))
	out.Write(crlBuf.Bytes())
}

// writeEOMChunk re-appends the standard, empty-payload EOM terminator.
func writeEOMChunk(out *bytes.Buffer) {
	out.WriteString(string(EOM))
	binary.Write(out, binary.BigEndian, int32(0))
}

// Pass2 runs the Typer flow analysis pass and outputs the modified HXB bytecode stream.
func (a *Annotator) Pass2() ([]byte, error) {
	a.TyperObj.SpecializeAndCheck(a.Nodes)
	a.Tags = a.TyperObj.Tags
	var out bytes.Buffer
	out.Write(a.Buf)
	out.Write([]byte("EOM"))
	writeCrLChunk(&out, a.Tags)
	writeEOMChunk(&out)
	return out.Bytes(), nil
}
