package main

import (
	"bytes"
	"fmt"
	"strings"
)

// Driver defines the behavioral requirements for target generation targets
type Driver interface {
	Mng(id int, t string) string            // Managed (Standard Target GC)
	Stk(id int, t string, off int32) string // Stack Frame (Offset-based)
	Hep(id int, t string) string            // Unmanaged Heap (Malloc)
	Inl(id int, t string, off int32) string // Inlined (Packed child structure)
	Free(id int, place byte) string         // Lifespan termination clean-up
}

type Writer struct {
	lang Driver
}

func NewWriter(d Driver) *Writer {
	return &Writer{lang: d}
}

// WriteBlock loops over your typed variables and builds the exact syntax stream
func (w *Writer) WriteBlock(vars []Track) string {
	var sb strings.Builder

	for _, t := range vars {
		switch t.Place {
		case Managed:
			sb.WriteString(w.lang.Mng(t.ID, t.Type) + "\n")
		case Stack:
			sb.WriteString(w.lang.Stk(t.ID, t.Type, t.Off) + "\n")
			sb.WriteString(w.lang.Free(t.ID, Stack) + "\n")
		case UnmanagedHeap:
			sb.WriteString(w.lang.Hep(t.ID, t.Type) + "\n")
			sb.WriteString(w.lang.Free(t.ID, UnmanagedHeap) + "\n")
		case Inlined:
			sb.WriteString(w.lang.Inl(t.ID, t.Type, t.Off) + "\n")
		}
	}
	return sb.String()
}

// writeUleb128 encodes v as an unsigned LEB128 varint, matching
// Reader.readUleb128's wire format exactly. Used by Annotator.Pass2's crL
// emission (see writeCrLChunk in Annotator.go), and available to any other
// chunk-writing pass that needs the same encoding.
func writeUleb128(buf *bytes.Buffer, v int) {
	u := uint(v)
	for {
		b := byte(u & 0x7F)
		u >>= 7
		if u != 0 {
			buf.WriteByte(b | 0x80)
		} else {
			buf.WriteByte(b)
			break
		}
	}
}

// BaseDriver is the composite-embedding chassis every target-language
// driver builds on: it gives sane fallback syntax for all five Driver
// methods, so a new language (Zig, D, Nim, Beef, ...) only needs to embed
// BaseDriver and override the handful of methods where its syntax actually
// differs, rather than implementing Driver from scratch each time.
type BaseDriver struct{}

func (BaseDriver) Mng(id int, t string) string {
	return fmt.Sprintf("var v%d: %s;", id, t)
}
func (BaseDriver) Stk(id int, t string, off int32) string {
	return fmt.Sprintf("var v%d: %s; // stack @%d", id, t, off)
}
func (BaseDriver) Hep(id int, t string) string {
	return fmt.Sprintf("var v%d: %s; // heap-alloc", id, t)
}
func (BaseDriver) Inl(id int, t string, off int32) string {
	return fmt.Sprintf("var v%d: %s; // inline @%d", id, t, off)
}
func (BaseDriver) Free(id int, place byte) string {
	return fmt.Sprintf("free(v%d);", id)
}

// ZigDriver is a minimal example of the pattern: embed BaseDriver, override
// only what Zig's syntax actually requires to differ. DDriver, NimDriver,
// BeefDriver, etc. would follow the same shape.
type ZigDriver struct{ BaseDriver }

func (ZigDriver) Stk(id int, t string, off int32) string {
	return fmt.Sprintf("var v%d: %s = undefined; // stack @%d", id, t, off)
}
func (ZigDriver) Free(id int, place byte) string {
	return fmt.Sprintf("defer v%d.deinit();", id)
}
