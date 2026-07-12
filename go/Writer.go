package main

import "strings"

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
