package main

import (
	"bytes"
	"fmt"
	"strings"
)

// Driver defines the behavioral requirements for target generation targets
type Driver interface {
	Managed(id int, typeStr string) string
	Stack(id int, typeStr string, offset int32) string
	Heap(id int, typeStr string) string
	Inlined(id int, typeStr string, offset int32) string
	Free(id int, place MemoryTarget) string // Standardized to MemoryTarget
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
			sb.WriteString(w.lang.Managed(t.ID, t.Type) + "\n")
		case Stack:
			sb.WriteString(w.lang.Stack(t.ID, t.Type, t.Off) + "\n")
			sb.WriteString(w.lang.Free(t.ID, Stack) + "\n")
		case UnmanagedHeap:
			sb.WriteString(w.lang.Heap(t.ID, t.Type) + "\n")
			sb.WriteString(w.lang.Free(t.ID, UnmanagedHeap) + "\n")
		case Inlined:
			sb.WriteString(w.lang.Inlined(t.ID, t.Type, t.Off) + "\n")
		}
	}
	return sb.String()
}

// writeUleb128 encodes v as an unsigned LEB128 varint, matching
// Reader.readUleb128's wire format exactly.
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

// BaseDriver provides sane fallback syntax for all code generation methods,
// allowing target implementations to override only what they need.
type BaseDriver struct{}

func (BaseDriver) Managed(id int, typeStr string) string {
	return fmt.Sprintf("var v%d: %s;", id, typeStr)
}

func (BaseDriver) Stack(id int, typeStr string, offset int32) string {
	return fmt.Sprintf("var v%d: %s; // stack @%d", id, typeStr, offset)
}

func (BaseDriver) Heap(id int, typeStr string) string {
	return fmt.Sprintf("var v%d: %s; // heap-alloc", id, typeStr)
}

func (BaseDriver) Inlined(id int, typeStr string, offset int32) string {
	return fmt.Sprintf("var v%d: %s; // inline @%d", id, typeStr, offset)
}

func (BaseDriver) Free(id int, place MemoryTarget) string {
	return fmt.Sprintf("free(v%d);", id)
}

// ZigDriver inherits default templates from BaseDriver and overrides
// specialized allocation defaults and lazy memory cleanup expressions.
type ZigDriver struct{ BaseDriver }

func (ZigDriver) Stack(id int, typeStr string, offset int32) string {
	return fmt.Sprintf("var v%d: %s = undefined; // stack @%d", id, typeStr, offset)
}

func (ZigDriver) Free(id int, place MemoryTarget) string {
	return fmt.Sprintf("defer v%d.deinit();", id)
}
