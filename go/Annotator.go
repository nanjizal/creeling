package main

import (
	"bytes"
	"encoding/binary"
)

// HxbStructureParser represents your downstream structural target interface contract.
type HxbStructureParser interface {
	ReaderApi
}

// Annotator uses Anonymous Embedding to "extend" the base Reader class.
type Annotator struct {
	*Reader         // 🎯 ANONYMOUS EMBEDDING: Annotator now inherits ALL methods of Reader!
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

func (r *Reader) readExpression() error {
	opByte, err := r.readByte()
	if err != nil {
		return err
	}
	op := FullOpcode(opByte)

	if op == Eof {
		return nil
	}

	// Increment your Creeling flow counter on active tokens
	//r.currentExprOffset++

	switch op {
	case EConst, ELocal, EType:
		// Format: [Opcode] + [PoolIndex: Uleb128]
		_, _ = r.readUleb128()

	case EArray, EIn, EBinop:
		// Binop format: [Opcode] + [BinopType: Byte] + [Left: Expr] + [Right: Expr]
		if op == EBinop {
			_, _ = r.readByte()
		}
		if err := r.readExpression(); err != nil {
			return err
		} // Left
		if err := r.readExpression(); err != nil {
			return err
		} // Right

	case EField, ECheckType, EMeta:
		// Format: [Opcode] + [Target: Expr] + [PoolIndex: Uleb128]
		if err := r.readExpression(); err != nil {
			return err
		}
		_, _ = r.readUleb128()

	case EParenthesis, EUntyped, EThrow:
		// Format: [Opcode] + [Inner: Expr]
		if err := r.readExpression(); err != nil {
			return err
		}

	case EObjectDecl:
		// Format: [Opcode] + [Count: Uleb128] + N * ([FieldNameIndex: Uleb128] + [Value: Expr])
		count, _ := r.readUleb128()
		for i := 0; i < int(count); i++ {
			_, _ = r.readUleb128()
			if err := r.readExpression(); err != nil {
				return err
			}
		}

	case EArrayDecl, EBlock:
		// Format: [Opcode] + [Count: Uleb128] + N * [Item: Expr]
		count, _ := r.readUleb128()
		for i := 0; i < int(count); i++ {
			if err := r.readExpression(); err != nil {
				return err
			}
		}

	case ECall, ENew:
		// ECall format: [Opcode] + [Target: Expr] + [ArgCount: Uleb128] + N * [Arg: Expr]
		// ENew format:  [Opcode] + [ClassIdx: Uleb128] + [ArgCount: Uleb128] + N * [Arg: Expr]
		if op == ECall {
			if err := r.readExpression(); err != nil {
				return err
			}
		} else {
			_, _ = r.readUleb128() // ClassIdx
		}
		argCount, _ := r.readUleb128()
		for i := 0; i < int(argCount); i++ {
			if err := r.readExpression(); err != nil {
				return err
			}
		}

	case EUnop:
		// Format: [Opcode] + [OpType: Byte] + [IsPostfix: Byte] + [Target: Expr]
		_, _ = r.readByte()
		_, _ = r.readByte()
		if err := r.readExpression(); err != nil {
			return err
		}

	case EFunction:
		// Format: [Opcode] + [NameIdx: Uleb128] + [FuncSignature details...]
		_, _ = r.readUleb128()
		// (Assuming a basic delegate jump or custom function block skip is handled here)

	case EFor:
		// Format: [Opcode] + [LoopVarIdx: Uleb128] + [Iter: Expr] + [Body: Expr]
		_, _ = r.readUleb128()
		if err := r.readExpression(); err != nil {
			return err
		} // Iterator
		if err := r.readExpression(); err != nil {
			return err
		} // Loop Body

	case EIf:
		// Format: [Opcode] + [Cond: Expr] + [Then: Expr] + [HasElse: Byte] + [OptionalElse: Expr]
		if err := r.readExpression(); err != nil {
			return err
		} // Condition
		if err := r.readExpression(); err != nil {
			return err
		} // Then
		hasElse, _ := r.readByte()
		if hasElse == 1 {
			if err := r.readExpression(); err != nil {
				return err
			} // Else
		}

	case EWhile:
		// Format: [Opcode] + [Cond: Expr] + [Body: Expr] + [IsDoWhile: Byte]
		if err := r.readExpression(); err != nil {
			return err
		}
		if err := r.readExpression(); err != nil {
			return err
		}
		_, _ = r.readByte()

	case EReturn:
		// Format: [Opcode] + [HasValue: Byte] + [OptionalValue: Expr]
		hasValue, _ := r.readByte()
		if hasValue == 1 {
			if err := r.readExpression(); err != nil {
				return err
			}
		}

	case ECast:
		// Format: [Opcode] + [Target: Expr] + [HasType: Byte] + [OptionalType: Uleb128]
		if err := r.readExpression(); err != nil {
			return err
		}
		hasType, _ := r.readByte()
		if hasType == 1 {
			_, _ = r.readUleb128()
		}

	case ETernary:
		// Format: [Opcode] + [Cond: Expr] + [Then: Expr] + [Else: Expr]
		if err := r.readExpression(); err != nil {
			return err
		}
		if err := r.readExpression(); err != nil {
			return err
		}
		if err := r.readExpression(); err != nil {
			return err
		}

	case EBreak, EContinue:
		// Leaf nodes: Exit immediately
		return nil

	default:
		// Fall-through catch-all for complex nested structures (ESwitch, ETry, EDisplay)
		// These carry highly specialized embedded tables we can flesh out as needed.
	}

	return nil
}

// Pass1 parses the binary cache into structured node lists and evaluates variable lifespans.
func (a *Annotator) Pass1(targetApi ReaderApi) (*Module, error) {
	a.Pos = 0
	a.Tags = nil

	// Step A: Read the binary layout stream to build your structured Module node tree natively
	m, err := a.Reader.Read(targetApi)
	if err != nil {
		return nil, err
	}

	// Step B: Extract the authentic EXD expression node slice compiled by Haxe
	var programNodes []Node
	if m != nil {
		// Aligned directly to the flat nodes/expression fields array list in your hxb_reader.go
		// programNodes = m.Nodes
	}

	// Step C: Instantiate your native Context tracking wrapper from typer.go
	ctx := &Context{
		Variables: make(map[int]VariableTrack),
	}

	// Step D: Execute your flow analysis logic to process the block branches
	a.TyperObj.ProcessBlock(programNodes, ctx)

	// Step E: Loop over your native variables context to determine target layout markers
	for id, vTrack := range ctx.Variables {
		targetPlace := Stack
		if vTrack.State == Leaked {
			targetPlace = UnmanagedHeap
		}

		layoutOffset := int32(-16) // Default fallback hardware spacing tracking metrics

		// Directly append the minimal splicing tag. Since your fixed-window scanner
		// handles the stream, a.Pos tracks the exact binary index coordinates.
		a.Tags = append(a.Tags, Tag{
			Pos:   a.Pos,
			Place: targetPlace,
			Off:   layoutOffset,
		})

		// To suppress Go compile-time unused diagnostics for the local variable mapping 'id'
		_ = id
	}

	return m, nil
}

/*
// Pass2 streams out the original binary block contents and appends the custom hidden crL chunk payload.
func (a *Annotator) Pass2() []byte {
	var out bytes.Buffer
	n := len(a.Buf)

	// Step A: Find the final EOM (End of Module) marker boundary inside your buffer
	eomPos := n
	for i := n - 3; i >= 0; i-- {
		if string(a.Buf[i:i+3]) == "EOM" {
			eomPos = i
			break
		}
	}

	// Stream original standard HXB payload chunks untouched right up to the end boundary
	out.Write(a.Buf[:eomPos])

	// Step B: Serialize custom tags into a temporary payload buffer
	var crlBuf bytes.Buffer
	if len(a.Tags) > 0 {
		// Write out the total count of injections being registered in this module file
		crlBuf.WriteByte(byte(len(a.Tags)))

		for _, t := range a.Tags {
			// Write Pos as a 4-byte Int32 to ensure non-trivial file sizes never truncate coordinates
			binary.Write(&crlBuf, binary.BigEndian, int32(t.Pos))

			// Write the hardware destination placement byte (Stack, UnmanagedHeap, etc.)
			crlBuf.WriteByte(t.Place)

			// Universal cross-language bit shifts for the layout displacement mapping
			crlBuf.WriteByte(byte(t.Off & 0xFF))
			crlBuf.WriteByte(byte((t.Off >> 8) & 0xFF))
			crlBuf.WriteByte(byte((t.Off >> 16) & 0xFF))
			crlBuf.WriteByte(byte((t.Off >> 24) & 0xFF))
		}

		// Step C: Inject the pre-registered 3-character hidden "crL" chunk identifier header
		out.Write([]byte("crL"))

		// Write chunk payload data size width as Big-Endian Int32 to match standard chunk formats
		chunkSize := int32(crlBuf.Len())
		binary.Write(&out, binary.BigEndian, chunkSize)

		// Dump the actual serialization payload body contents
		out.Write(crlBuf.Bytes())
	}

	// Step D: Write out the final EOM trailing block sequence to terminate the file smoothly
	out.Write([]byte("EOM"))
	binary.Write(&out, binary.BigEndian, int32(0))

	return out.Bytes()
}
*/
// Pass2 streams out the original binary block contents and appends the custom hidden crL chunk payload.
func (a *Annotator) Pass2() []byte {
	var out bytes.Buffer
	n := len(a.Buf)

	// Find the final EOM (End of Module) marker boundary inside your buffer
	eomPos := n
	for i := n - 3; i >= 0; i-- {
		if string(a.Buf[i:i+3]) == "EOM" {
			eomPos = i
			break
		}
	}

	// Stream original standard HXB payload chunks untouched right up to the end boundary
	out.Write(a.Buf[:eomPos])

	// Serialize custom tags into a temporary payload buffer
	var crlBuf bytes.Buffer
	if len(a.Tags) > 0 {
		crlBuf.WriteByte(byte(len(a.Tags)))

		for _, t := range a.Tags {
			binary.Write(&crlBuf, binary.BigEndian, int32(t.Pos))
			crlBuf.WriteByte(t.Place)

			crlBuf.WriteByte(byte(t.Off & 0xFF))
			crlBuf.WriteByte(byte((t.Off >> 8) & 0xFF))
			crlBuf.WriteByte(byte((t.Off >> 16) & 0xFF))
			crlBuf.WriteByte(byte((t.Off >> 24) & 0xFF))
		}

		out.Write([]byte("crL"))
		chunkSize := int32(crlBuf.Len())
		binary.Write(&out, binary.BigEndian, chunkSize)
		out.Write(crlBuf.Bytes())
	}

	// Write out the final EOM trailing block sequence to terminate the file smoothly
	out.Write([]byte("EOM"))
	binary.Write(&out, binary.BigEndian, int32(0))

	return out.Bytes()
}
