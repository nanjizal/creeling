package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// HxbStructureParser represents your downstream structural target interface contract.
type HxbStructureParser interface {
	ReaderApi
}

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

// readExpression decodes one expression from the EXD/EFD/AFD stream and
// builds the real Node tree the Typer consumes (VarID, Nodes, ThenBlock,
// ElseBlock, and now Offset). This is a *Reader method physically defined
// here in Annotator.go — Go allows splitting a type's methods across files
// in the same package, so this needs no back-pointer/interface seam to
// reach Reader's primitives (readByte/readUleb128/currentExprOffset/etc).
//
// EDIT: previously this only advanced the byte cursor and discarded
// everything (a pure "skip parser") — every read call below is in the
// exact same order as that version, this is additive only, not a format
// change.
func (r *Reader) readExpression() (Node, error) {
	offset := r.currentExprOffset
	r.currentExprOffset++

	opByte, err := r.readByte()
	if err != nil {
		return Node{}, err
	}
	op := FullOpcode(opByte)
	node := Node{Kind: op, Offset: offset}

	if op == Eof {
		return node, nil
	}

	switch op {
	case EConst, EType:
		// Format: [Opcode] + [PoolIndex: Uleb128]
		_, err = r.readUleb128()
		return node, err

	case ELocal:
		// Format: [Opcode] + [PoolIndex: Uleb128] — the pool index doubles as VarID here
		idx, err := r.readUleb128()
		if err != nil {
			return node, err
		}
		node.VarID = idx
		return node, nil

	case EArray, EIn, EBinop:
		// Binop format: [Opcode] + [BinopType: Byte] + [Left: Expr] + [Right: Expr]
		if op == EBinop {
			if _, err := r.readByte(); err != nil {
				return node, err
			}
		}
		left, err := r.readExpression()
		if err != nil {
			return node, err
		}
		right, err := r.readExpression()
		if err != nil {
			return node, err
		}
		node.Nodes = []Node{left, right}
		if left.VarID != 0 {
			node.VarID = left.VarID
		}
		return node, nil

	case EField, ECheckType, EMeta:
		// Format: [Opcode] + [Target: Expr] + [PoolIndex: Uleb128]
		target, err := r.readExpression()
		if err != nil {
			return node, err
		}
		if _, err := r.readUleb128(); err != nil {
			return node, err
		}
		node.Nodes = []Node{target}
		node.VarID = target.VarID
		return node, nil

	case EParenthesis, EUntyped, EThrow:
		// Format: [Opcode] + [Inner: Expr]
		inner, err := r.readExpression()
		if err != nil {
			return node, err
		}
		node.Nodes = []Node{inner}
		node.VarID = inner.VarID
		return node, nil

	case EObjectDecl:
		// Format: [Opcode] + [Count: Uleb128] + N * ([FieldNameIndex: Uleb128] + [Value: Expr])
		count, err := r.readUleb128()
		if err != nil {
			return node, err
		}
		for i := 0; i < count; i++ {
			if _, err := r.readUleb128(); err != nil {
				return node, err
			}
			val, err := r.readExpression()
			if err != nil {
				return node, err
			}
			node.Nodes = append(node.Nodes, val)
		}
		return node, nil

	case EArrayDecl, EBlock:
		// Format: [Opcode] + [Count: Uleb128] + N * [Item: Expr]
		count, err := r.readUleb128()
		if err != nil {
			return node, err
		}
		for i := 0; i < count; i++ {
			child, err := r.readExpression()
			if err != nil {
				return node, err
			}
			node.Nodes = append(node.Nodes, child)
		}
		return node, nil

	case ECall, ENew:
		// ECall format: [Opcode] + [Target: Expr] + [ArgCount: Uleb128] + N * [Arg: Expr]
		// ENew format:  [Opcode] + [ClassIdx: Uleb128] + [ArgCount: Uleb128] + N * [Arg: Expr]
		var target Node
		if op == ECall {
			target, err = r.readExpression()
			if err != nil {
				return node, err
			}
			node.VarID = target.VarID
		} else {
			if _, err := r.readUleb128(); err != nil { // ClassIdx
				return node, err
			}
		}
		argCount, err := r.readUleb128()
		if err != nil {
			return node, err
		}
		args := make([]Node, 0, argCount)
		for i := 0; i < argCount; i++ {
			arg, err := r.readExpression()
			if err != nil {
				return node, err
			}
			args = append(args, arg)
		}
		if op == ECall {
			node.Nodes = append([]Node{target}, args...)
		} else {
			node.Nodes = args
		}
		return node, nil

	case EUnop:
		// Format: [Opcode] + [OpType: Byte] + [IsPostfix: Byte] + [Target: Expr]
		if _, err := r.readByte(); err != nil {
			return node, err
		}
		if _, err := r.readByte(); err != nil {
			return node, err
		}
		target, err := r.readExpression()
		if err != nil {
			return node, err
		}
		node.Nodes = []Node{target}
		node.VarID = target.VarID
		return node, nil

	case EFunction:
		// Format: [Opcode] + [NameIdx: Uleb128] + [FuncSignature details...]
		if _, err := r.readUleb128(); err != nil {
			return node, err
		}
		return node, nil

	case EFor:
		// Format: [Opcode] + [LoopVarIdx: Uleb128] + [Iter: Expr] + [Body: Expr]
		if _, err := r.readUleb128(); err != nil {
			return node, err
		}
		iter, err := r.readExpression()
		if err != nil {
			return node, err
		}
		body, err := r.readExpression()
		if err != nil {
			return node, err
		}
		node.Nodes = []Node{iter, body}
		return node, nil

	case EIf:
		// Format: [Opcode] + [Cond: Expr] + [Then: Expr] + [HasElse: Byte] + [OptionalElse: Expr]
		cond, err := r.readExpression()
		if err != nil {
			return node, err
		}
		thenExpr, err := r.readExpression()
		if err != nil {
			return node, err
		}
		node.Nodes = []Node{cond}
		node.ThenBlock = []Node{thenExpr}
		hasElse, err := r.readByte()
		if err != nil {
			return node, err
		}
		if hasElse == 1 {
			elseExpr, err := r.readExpression()
			if err != nil {
				return node, err
			}
			node.ElseBlock = []Node{elseExpr}
		}
		return node, nil

	case EWhile:
		// Format: [Opcode] + [Cond: Expr] + [Body: Expr] + [IsDoWhile: Byte]
		cond, err := r.readExpression()
		if err != nil {
			return node, err
		}
		body, err := r.readExpression()
		if err != nil {
			return node, err
		}
		if _, err := r.readByte(); err != nil {
			return node, err
		}
		node.Nodes = []Node{cond, body}
		return node, nil

	case EReturn:
		// Format: [Opcode] + [HasValue: Byte] + [OptionalValue: Expr]
		hasValue, err := r.readByte()
		if err != nil {
			return node, err
		}
		if hasValue == 1 {
			val, err := r.readExpression()
			if err != nil {
				return node, err
			}
			node.Nodes = []Node{val}
			node.VarID = val.VarID
		}
		return node, nil

	case ECast:
		// Format: [Opcode] + [Target: Expr] + [HasType: Byte] + [OptionalType: Uleb128]
		target, err := r.readExpression()
		if err != nil {
			return node, err
		}
		node.Nodes = []Node{target}
		node.VarID = target.VarID
		hasType, err := r.readByte()
		if err != nil {
			return node, err
		}
		if hasType == 1 {
			if _, err := r.readUleb128(); err != nil {
				return node, err
			}
		}
		return node, nil

	case ETernary:
		// Format: [Opcode] + [Cond: Expr] + [Then: Expr] + [Else: Expr]
		cond, err := r.readExpression()
		if err != nil {
			return node, err
		}
		thenExpr, err := r.readExpression()
		if err != nil {
			return node, err
		}
		elseExpr, err := r.readExpression()
		if err != nil {
			return node, err
		}
		node.Nodes = []Node{cond}
		node.ThenBlock = []Node{thenExpr}
		node.ElseBlock = []Node{elseExpr}
		return node, nil

	case EBreak, EContinue:
		// Leaf nodes: Exit immediately
		return node, nil

	default:
		// Fall-through catch-all for complex nested structures (ESwitch, ETry, EDisplay)
		// These carry highly specialized embedded tables we can flesh out as needed.
		return node, nil
	}
}

// Pass1 parses the binary stream into the real Node tree (via Reader.Read,
// which now dispatches EFD/EXD into readExpression thanks to the whitelist
// fix in hxb_reader.go) and caches it on the Annotator. It does NOT run the
// Typer — that now happens at the start of Pass2, which stashes its
// Context on TyperObj.Ctx so Pass2 can build Tags from it directly.
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

// findEOMOffset locates the EOM chunk the same tolerant way Reader.Read
// parses the whole file: scan for a valid chunk name, read its size, and
// if the cursor drifts (an unrecognized or misaligned chunk), re-scan
// forward byte-by-byte for the next valid anchor rather than trusting size
// math blindly. This deliberately mirrors Read()'s self-healing window
// scanner, since real hxb.cross output can hit exactly the drift it
// recovers from — a plain substring search for "EOM" would not share that
// tolerance and could misalign against real files.
func findEOMOffset(buf []byte) (int, error) {
	pos := 4 // skip "hxb\x01" magic
	window := make([]byte, 3)

	for pos+3 <= len(buf) {
		copy(window, buf[pos:pos+3])
		nameStr := string(window)

		if !validChunks[nameStr] {
			// Drift recovery: slide forward one byte at a time until the
			// window lands back on a recognized chunk name.
			found := false
			for p := pos + 1; p+3 <= len(buf); p++ {
				copy(window, buf[p:p+3])
				if validChunks[string(window)] {
					pos = p
					nameStr = string(window)
					found = true
					break
				}
			}
			if !found {
				return 0, fmt.Errorf("%w: EOM chunk not found", ErrHxbReaderException)
			}
		}

		if pos+7 > len(buf) {
			return 0, fmt.Errorf("%w: truncated chunk header near offset %d", ErrHxbReaderException, pos)
		}
		if ChunkKind(nameStr) == EOM {
			return pos, nil
		}

		size := int(binary.BigEndian.Uint32(buf[pos+3 : pos+7]))
		pos += 7 + size
	}

	return 0, fmt.Errorf("%w: EOM chunk not found", ErrHxbReaderException)
}

// writeCrLChunk serializes tags into a fully-framed crL chunk (3-byte name,
// 4-byte big-endian size, then payload) matching the exact wire format
// hxb_reader.readcrL/applyLinearityState expects: uleb128(count), then per
// entry uleb128(pos), uleb128(varID), byte(place). Correct framing is what
// lets a generic hxb reader that has never heard of "crL" skip straight
// past it via its own size-based skip logic and land exactly on EOM.
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

// Pass2 runs the Typer's flow analysis over the nodes Pass1 parsed
// (SpecializeAndCheck stashes its Context on TyperObj.Ctx), builds Tags
// from the result, then re-emits the original chunk stream up to EOM
// followed by a crL chunk carrying those tags, then EOM.
//
// NOTE: signature changed from `func (a *Annotator) Pass2() []byte` to
// return an error too, since findEOMOffset can now genuinely fail instead
// of silently defaulting to "append at the end". Update call sites (e.g.
// main.go: `hxbPlusBytes, err := annot.Pass2()`) accordingly.
func (a *Annotator) Pass2() ([]byte, error) {
	a.TyperObj.SpecializeAndCheck(a.Nodes)

	a.Tags = nil
	for id, vTrack := range a.TyperObj.Ctx.Variables {
		a.Tags = append(a.Tags, Tag{
			Pos:   vTrack.Offset,
			VarID: id,
			Place: byte(vTrack.State),
		})
	}

	eomPos, err := findEOMOffset(a.Buf)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.Write(a.Buf[:eomPos])
	writeCrLChunk(&out, a.Tags)
	writeEOMChunk(&out)

	return out.Bytes(), nil
}
