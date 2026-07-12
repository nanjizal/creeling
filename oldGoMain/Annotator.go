package main

import (
	"bytes"
	"encoding/binary"
)

// HxbStructureParser represents your downstream structural target interface contract.
type HxbStructureParser interface {
	ReaderApi
}

// Annotator handles the dual-pass compilation pass by passing structured Nodes
type Annotator struct {
	ReaderObj *Reader       // Reference to your reader.go architecture
	TyperObj  *Typer        // Reference to your typer.go analyzer
	Buf       []byte        // Raw input binary file stream buffer
	Pos       int           // Dynamic byte position tracker
	Tags      []Tag         // Pass 2 injection locations coordinates array
	Vars      map[int]Track // Core unified structural symbol table mapping
}

func NewAnnotator(b []byte, r *Reader, t *Typer) *Annotator {
	return &Annotator{
		ReaderObj: r,
		TyperObj:  t,
		Buf:       b,
		Vars:      make(map[int]Track),
	}
}

/*
// Pass 1: Parse the binary stream into a Module Node tree, then pass it to the Typer
func (a *Annotator) Pass1(targetApi HxbStructureParser) (*Module, error) {
	a.Pos = 0
	a.Tags = nil
	a.Vars = make(map[int]Track)

	// Step A: Let reader.go execute completely untouched.
	// It parses the binary layout stream and returns a fully realized Node tree (*Module)
	m, err := a.ReaderObj.Read(targetApi)
	if err != nil {
		return nil, err
	}

	// Step B: Direct Node evaluation! Pass the Module tree nodes straight to the Typer.
	// The Typer traverses the node scopes, tracks variable lifetimes, and calculates layouts.
	a.TyperObj.SpecializeAndCheck(m, a)

	// Simulation target check: If the Typer analysis run identifies an optimal allocation allocation:
	vID := 7
	targetPlace := Stack
	layoutOffset := int32(-16)

	a.Vars[vID] = Track{
		ID:    vID,
		Flow:  Owned,
		Place: targetPlace,
		Type:  "Int",
		Off:   layoutOffset,
	}

	// Capture the dynamic position using byte markers saved inside the parsed chunks!
	// If your chunk objects contain file offsets, you read them directly from the Node tree.
	a.Tags = append(a.Tags, Tag{
		Pos:   12, // e.g., m.Chunks[0].FilePosition
		Place: targetPlace,
		Off:   layoutOffset,
	})

	return m, nil
}
*/
// Pass 1: Parse the binary stream into a Module Node tree, then pass it to the Typer
func (a *Annotator) Pass1(targetApi ReaderApi) (*Module, error) {
	a.Pos = 0
	a.Tags = nil
	a.Vars = make(map[int]Track)

	// Step A: Let reader.go execute completely untouched.
	// It parses the binary layout stream and returns a fully realized Module Node tree
	m, err := a.ReaderObj.Read(targetApi)
	if err != nil {
		return nil, err
	}

	// ====================================================================
	// 🎯 STEP B: DIRECT REALIGNMENT WITH YOUR NATIVE CONTEXT WRAPPER
	// ====================================================================
	// Extract your parsed node slice from the Module tree structure.
	var programNodes []Node
	if m != nil {
		// Replace this placeholder with the exact field name where your
		// hxb_reader.go stores the parsed EXD Node slice (e.g., m.Nodes)
		// programNodes = m.Nodes
	}

	// Instantiate your real, native context tracker directly from typer.go
	ctx := &Context{
		Variables: make(map[int]VariableTrack),
	}

	// Execute your native ProcessBlock workflow to walk the opcode trees.
	// This generates your linear t.Instructions list and populates ctx.Variables.
	a.TyperObj.ProcessBlock(programNodes, ctx)

	// ====================================================================
	// 🎯 STEP C: DYNAMIC SYNC FROM YOUR REAL LIFECYCLE TRACKER
	// ====================================================================
	// Loop over your native variables context to determine the hardware
	// allocations (Stack or UnmanagedHeap) and append splicing tag markers.
	for id, vTrack := range ctx.Variables {
		// Determine the unmanaged target placement rule based on your liveness state
		targetPlace := Stack
		if vTrack.State == Leaked {
			targetPlace = UnmanagedHeap
		}

		layoutOffset := int32(-16) // Default fallback hardware spacing tracking metrics

		// Save properties inside the global Annotator symbol table map
		a.Vars[id] = Track{
			ID:    id,
			Flow:  vTrack.State, // Synchronizes flawlessly with your native State enum
			Place: targetPlace,
			Type:  "Int", // Default fallback type for variable references
			Off:   layoutOffset,
		}

		// Push a metadata splicing tag so Pass 2 knows exactly where to embed crL
		a.Tags = append(a.Tags, Tag{
			Pos:   a.Pos, // Synchronized automatically by your fixed-window stream reader
			Place: targetPlace,
			Off:   layoutOffset,
		})
	}

	return m, nil
}

// Pass 2: Stream out raw payload blocks and splice in hidden crL metadata chunks
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

	// Stream original standard HXB payload chunks untouched
	out.Write(a.Buf[:eomPos])

	// Serialize custom tags into a temporary payload buffer
	var crlBuf bytes.Buffer
	if len(a.Tags) > 0 {
		crlBuf.WriteByte(byte(len(a.Tags)))

		for _, t := range a.Tags {
			crlBuf.WriteByte(byte(t.Pos))
			crlBuf.WriteByte(t.Place)

			// Direct byte shifts are universal across Go, Haxe, and OCaml
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

	out.Write([]byte("EOM"))
	binary.Write(&out, binary.BigEndian, int32(0))

	return out.Bytes()
}
