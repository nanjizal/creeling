package main

// From Format haxelib, see PXshadow's wip-hxb branch.
/*
BSD 2-Clause License

Copyright (c) 2008-2024, Haxe Foundation

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

import (
	"errors"
	"fmt"
)

// HxbReaderException maps directly to the custom Haxe Exception definition
var ErrHxbReaderException = errors.New("HxbReaderException")

// Primitives

type Todo struct{}

type NoData struct{}

func NewNoData() NoData {
	return NoData{}
}

// Compounds

type Path struct {
	Pack []string `haxe:"pack"`
	Name string   `haxe:"name"`
}

type FullPath struct {
	Path     `haxe:"extends"` // Haxe & composition intersection
	TypeName string           `haxe:"typeName"`
}

type Pos struct {
	File string `haxe:"file"`
	Min  int    `haxe:"min"`
	Max  int    `haxe:"max"`
}

// Option wrapper pattern to mirror Haxe's haxe.ds.Option<T> cleanly
type OptionClassField struct {
	HasValue bool
	Value    ClassField
}

type OptionTypeInstance struct {
	HasValue bool
	Value    *TypeInstance
}

// Module types

type ModuleTypeKindType int

const (
	KindClass ModuleTypeKindType = iota
	KindEnum
	KindTypedef
	KindAbstract
)

// ModuleTypeKind acts as an algebraic data type (enum with data structures)
type ModuleTypeKind struct {
	Kind     ModuleTypeKindType
	Class    ClassData    // Filled if Kind == KindClass
	Enum     EnumData     // Filled if Kind == KindEnum
	Typedef  TypedefData  // Filled if Kind == KindTypedef
	Abstract AbstractData // Filled if Kind == KindAbstract
}

type ModuleType struct {
	Path    Path            `haxe:"path"`
	Pos     Pos             `haxe:"pos"`
	NamePos Pos             `haxe:"namePos"`
	Params  []TypeParameter `haxe:"params"` // Go slices map smoothly to Haxe Vectors
}

type ModuleTypeWithKind struct {
	M    ModuleType     `haxe:"m"`
	Kind ModuleTypeKind `haxe:"kind"`
}

type ClassData struct {
	Flags       int              `haxe:"flags"`
	Constructor OptionClassField `haxe:"constructor"`
	Fields      []ClassField     `haxe:"fields"`
	Statics     []ClassField     `haxe:"statics"`
	Init        OptionClassField `haxe:"init"`
}

type EnumData struct {
	Fields []EnumField `haxe:"fields"`
}

type TypedefData struct{}
type AbstractData struct{}

type Class struct {
	M ModuleType `haxe:"m"`
	C ClassData  `haxe:"c"`
}

type Enum struct {
	M  ModuleType `haxe:"m"`
	En EnumData   `haxe:"en"`
}

type Typedef struct {
	M  ModuleType  `haxe:"m"`
	Td TypedefData `haxe:"td"`
}

type Abstract struct {
	M ModuleType   `haxe:"m"`
	A AbstractData `haxe:"a"`
}

// Type parameters

type TypeParameterHost int

const (
	HostType TypeParameterHost = iota
	HostConstructor
	HostMethod
	HostEnumConstructor
	HostAnonField
	HostLocal
)

type TypeParameter struct {
	Path Path              `haxe:"path"`
	Pos  Pos               `haxe:"pos"`
	Host TypeParameterHost `haxe:"host"`
}

// Type instances

type Monomorph struct {
	Type OptionTypeInstance `haxe:"type"`
}

type TypeInstanceKind int

const (
	TypeTInst TypeInstanceKind = iota
	TypeTEnum
	TypeTAbstract
	TypeTType
	TypeTMono
	TypeTDynamic
	TypeTDynamicAccess
)

// TypeInstance maps the recursive enum structure from Haxe
type TypeInstance struct {
	Kind     TypeInstanceKind
	Class    *Class         // Filled if TInst
	Enum     *Enum          // Filled if TEnum
	Abstract *Abstract      // Filled if TAbstract
	Typedef  *Typedef       // Filled if TType
	Mono     Monomorph      // Filled if TMono
	Params   []TypeInstance // Used for Type parameters (e.g., Array<TInstance>)
	Child    *TypeInstance  // Used explicitly for TDynamicAccess wrapper reference
}

// Fields

type ClassField struct {
	Name      string       `haxe:"name"`
	Pos       Pos          `haxe:"pos"`
	NamePos   Pos          `haxe:"namePos"`
	Overloads []ClassField `haxe:"overloads"`
}

type EnumField struct {
	Name    string `haxe:"name"`
	Pos     Pos    `haxe:"pos"`
	NamePos Pos    `haxe:"namePos"`
	Index   int    `haxe:"index"`
}

// Chunks

type MDF struct {
	Path     Path   `haxe:"path"`
	File     string `haxe:"file"`
	NumAnons int    `haxe:"numAnons"`
	NumMonos int    `haxe:"numMonos"`
}

type MTF struct {
	Types []ModuleTypeWithKind `haxe:"types"`
}

type Module struct {
	Mdf MDF  `haxe:"MDF"`
	Mtf *MTF `haxe:"MTF"` // Nullable reference to capture Null<MTF> cleanly
}

// ChunkKind Enum Abstract

type ChunkKind string

const (
	STR       ChunkKind = "STR"
	DOC       ChunkKind = "DOC"
	MDF_Chunk ChunkKind = "MDF"
	MTF_Chunk ChunkKind = "MTF"
	CLR       ChunkKind = "CLR"
	ENR       ChunkKind = "ENR"
	ABR       ChunkKind = "ABR"
	TDR       ChunkKind = "TDR"
	OFR       ChunkKind = "OFR"
	CLD       ChunkKind = "CLD"
	END       ChunkKind = "END"
	ABD       ChunkKind = "ABD"
	TDD       ChunkKind = "TDD"
	EOT       ChunkKind = "EOT"
	EFR       ChunkKind = "EFR"
	CFR       ChunkKind = "CFR"
	CFD       ChunkKind = "CFD"
	EFD       ChunkKind = "EFD"
	AFD       ChunkKind = "AFD"
	OFD       ChunkKind = "OFD"
	EOF       ChunkKind = "EOF"
	EXD       ChunkKind = "EXD"
	EOM       ChunkKind = "EOM"
	IMP       ChunkKind = "IMP"
	OBD       ChunkKind = "OBD"

	// Pre-registered extension token so hxbPlus works out of the box
	CrLn_Chunk ChunkKind = "crL"
)

// A flat, fast registration lookup map
var validChunks = map[string]bool{
	"STR": true, "DOC": true, "MDF": true, "MTF": true,
	"CLR": true, "ENR": true, "ABR": true, "TDR": true,
	"OFR": true, "CLD": true, "END": true, "ABD": true,
	"TDD": true, "EOT": true, "EFR": true, "CFR": true,
	"CFD": true, "EFD": true, "AFD": true, "OFD": true,
	"EOF": true, "EXD": true, "EOM": true, "IMP": true,
	"OBD": true, "crL": true,
}

// ChunkKindFromString performs direct string casting validation
func ChunkKindFromString(s string) (ChunkKind, error) {
	fmt.Println(s) // Native Haxe "trace(s)" debug statement

	if !validChunks[s] {
		return "", fmt.Errorf("%w: Unknown chunk kind: |%s|", ErrHxbReaderException, s)
	}

	return ChunkKind(s), nil
}
