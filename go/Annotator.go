package main

import (
	"bytes"
	"encoding/binary"
)

// HxbStructureParser acts exactly like a Haxe interface extending ReaderApi:
// `interface HxbStructureParser extends ReaderApi {}`
// This inherits all required parsing methods statically without manually re-typing them.
type HxbStructureParser interface {
	ReaderApi
	// If the Annotator pass requires extra bespoke callback hooks in the future,
	// they can be declared right here.
}

// Annotator uses clean, explicit composition by reference rather than anonymous embedding.
// This structure maps directly to properties on a Haxe class or fields in an OCaml record structure.
type Annotator struct {
	ReaderObj *Reader       // Explicit link to your hxb_reader.go parsing engine
	TyperObj  *Typer        // Explicit link to your typer.go type flow analyzer
	Buf       []byte        // Raw immutable source file byte container input stream
	Pos       int           // Tracking index counter mapping absolute position
	Tags      []Tag         // Metadata splicing marker collection array
	Vars      map[int]Track // Unified compiler symbol table index lookup mapping
}

// NewAnnotator creates a fully static, explicitly referenced pipeline constructor.
func NewAnnotator(b []byte, r *Reader, t *Typer) *Annotator {
	return &Annotator{
		ReaderObj: r,
		TyperObj:  t,
		Buf:       b,
		Vars:      make(map[int]Track),
	}
}

// ReadB overrides low-level stream scanning to keep the physical file Pos synced.
func (a *Annotator) ReadB() (byte, error) {
	// Call the reader method explicitly through the reference variable
	b, err := a.ReaderObj.readByte()
	if err == nil {
		a.Pos++ // Advances physical index location during the file scan pass
	}
	return b, err
}

// Pass 1: Scan standard HXB data via the base reader and map out unmanaged layout definitions.
func (a *Annotator) Pass1(targetApi HxbStructureParser) (*Module, error) {
	// targetApi now matches the ReaderApi interface type requirements perfectly
	m, err := a.ReaderObj.Read(targetApi)
	if err != nil {
		return nil, err
	}

	// Simulation: Your liveness pruning code isolates an allocation (VarID 7).
	// It notes that it can safely live on the CPU Stack frame with a -32 byte constraint layout.
	vID := 7
	a.Vars[vID] = Track{
		ID:    vID,
		Flow:  Owned,
		Place: Stack,
		Type:  "Int",
		Off:   -32,
	}

	// Register the tag offset. We mock an injection position at byte 10.
	a.Tags = append(a.Tags, Tag{Pos: 10, Place: Stack, Off: -32})

	return m, nil
}

// Pass 2: Stream valid HXB data out and append hidden crL chunk elements inline.
func (a *Annotator) Pass2() []byte {
	var out bytes.Buffer
	n := len(a.Buf)

	// Step A: Find the final EOM (End of Module) marker boundary in the original buffer.
	eomPos := n
	for i := n - 3; i >= 0; i-- {
		if string(a.Buf[i:i+3]) == "EOM" {
			eomPos = i
			break
		}
	}

	// Stream out the original valid standard HXB payload chunks untouched
	out.Write(a.Buf[:eomPos])

	// Step B: Serialize custom tags into a temporary payload buffer to auto-calculate the chunk width.
	var crlBuf bytes.Buffer
	if len(a.Tags) > 0 {
		// Write out the amount of injections being registered in this module file
		crlBuf.WriteByte(byte(len(a.Tags)))

		for _, t := range a.Tags {
			crlBuf.WriteByte(byte(t.Pos))
			crlBuf.WriteByte(t.Place)

			// Simple, portable bit-shifting masking for the Int32 offset configuration.
			// This arithmetic maps 1:1 into Haxe and OCaml seamlessly.
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

		// Dump the actual payload body contents
		out.Write(crlBuf.Bytes())
	}

	// Step D: Write out the final EOM trailing block sequence to terminate the file smoothly
	out.Write([]byte("EOM"))
	binary.Write(&out, binary.BigEndian, int32(0))

	return out.Bytes()
}
