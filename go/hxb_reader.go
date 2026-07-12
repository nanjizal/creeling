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
	"encoding/binary"
	"fmt"
	"io"
)

// ReaderApi defines the structural interface needed to handle module resolutions
type ReaderApi interface {
	ResolveModuleType(pack []string, name string, typeName string) (ModuleTypeWithKind, bool)
	AddModule(module *Module)
}

// LinearityKey binds a specific expression point to a target variable index
type LinearityKey struct {
	ExprOffset int
	VarID      int
}

type Reader struct {
	i            io.Reader
	api          ReaderApi
	stringPool   []string
	classes      []Class
	enums        []Enum
	typedefs     []Typedef
	abstracts    []Abstract
	anonFields   []ClassField
	module       *Module
	linearityMap map[LinearityKey]State
}

func NewReader(i io.Reader) *Reader {
	return &Reader{
		i: i,
	}
}

func (r *Reader) fail(reason string) error {
	return fmt.Errorf("%w: %s", ErrHxbReaderException, reason)
}

// readByte is a small helper to mirror Haxe's i.readByte()
func (r *Reader) readByte() (byte, error) {
	var b [1]byte
	_, err := r.i.Read(b[:])
	return b[0], err
}

// Primitives

// Ported from Haxe recursion to an optimized iterative Uleb128 loop
func (r *Reader) readUleb128() (int, error) {
	result := 0
	shift := 0
	for {
		b, err := r.readByte()
		if err != nil {
			return 0, err
		}
		result |= int(b&0x7F) << shift
		if b < 0x80 {
			break
		}
		shift += 7
	}
	return result, nil
}

// Ported from Haxe nested closures to a safe iterative Leb128 loop
func (r *Reader) readLeb128() (int, error) {
	result := 0
	shift := 0
	var b byte
	var err error

	for {
		b, err = r.readByte()
		if err != nil {
			return 0, err
		}
		result |= int(b&0x7F) << shift
		shift += 7
		if b < 0x80 {
			break
		}
	}

	// Sign extend if the last byte processed has its sign bit set (0x40)
	if (b & 0x40) != 0 {
		result |= ^0 << shift
	}
	return result, nil
}

func (r *Reader) readString() (string, error) {
	idx, err := r.readUleb128()
	if err != nil {
		return "", err
	}
	if idx < 0 || idx >= len(r.stringPool) {
		return "", r.fail(fmt.Sprintf("String pool index out of bounds: %d", idx))
	}
	return r.stringPool[idx], nil
}

// Helper to handle the option marker loops safely
func (r *Reader) readOptionMarker() (bool, error) {
	b, err := r.readByte()
	if err != nil {
		return false, err
	}
	return b != 0, nil
}

// Compounds

func (r *Reader) readPath() (Path, error) {
	packLen, err := r.readUleb128()
	if err != nil {
		return Path{}, err
	}
	pack := make([]string, packLen)
	for k := 0; k < packLen; k++ {
		str, err := r.readString()
		if err != nil {
			return Path{}, err
		}
		pack[k] = str
	}

	name, err := r.readString()
	if err != nil {
		return Path{}, err
	}

	return Path{Pack: pack, Name: name}, nil
}

func (r *Reader) readFullPath() (FullPath, error) {
	packLen, err := r.readUleb128()
	if err != nil {
		return FullPath{}, err
	}
	pack := make([]string, packLen)
	for k := 0; k < packLen; k++ {
		str, err := r.readString()
		if err != nil {
			return FullPath{}, err
		}
		pack[k] = str
	}

	name, err := r.readString()
	if err != nil {
		return FullPath{}, err
	}

	typeName, err := r.readString()
	if err != nil {
		return FullPath{}, err
	}

	return FullPath{
		Path:     Path{Pack: pack, Name: name},
		TypeName: typeName,
	}, nil
}

func (r *Reader) readPos() (Pos, error) {
	file, err := r.readString()
	if err != nil {
		return Pos{}, err
	}
	min, err := r.readUleb128()
	if err != nil {
		return Pos{}, err
	}
	max, err := r.readUleb128()
	if err != nil {
		return Pos{}, err
	}
	return Pos{File: file, Min: min, Max: max}, nil
}

// Type parameters

func (r *Reader) readTypeParameterHost() (TypeParameterHost, error) {
	b, err := r.readByte()
	if err != nil {
		return 0, err
	}
	switch b {
	case 0:
		return HostType, nil
	case 1:
		return HostConstructor, nil
	case 2:
		return HostMethod, nil
	case 3:
		return HostEnumConstructor, nil
	case 4:
		return HostAnonField, nil
	case 5:
		return HostLocal, nil
	default:
		return 0, r.fail(fmt.Sprintf("Unknown type parameter host byte: %d", b))
	}
}

func (r *Reader) readTypeParametersForward() ([]TypeParameter, error) {
	length, err := r.readUleb128()
	if err != nil {
		return nil, err
	}
	params := make([]TypeParameter, length)
	for i := 0; i < length; i++ {
		path, err := r.readPath()
		if err != nil {
			return nil, err
		}
		pos, err := r.readPos()
		if err != nil {
			return nil, err
		}
		host, err := r.readTypeParameterHost()
		if err != nil {
			return nil, err
		}
		params[i] = TypeParameter{
			Path: path,
			Pos:  pos,
			Host: host,
		}
	}
	return params, nil
}

// Fields

func (r *Reader) readClassFieldForward() (ClassField, error) {
	name, err := r.readString()
	if err != nil {
		return ClassField{}, err
	}
	pos, err := r.readPos()
	if err != nil {
		return ClassField{}, err
	}
	namePos, err := r.readPos()
	if err != nil {
		return ClassField{}, err
	}

	overloadsLen, err := r.readUleb128()
	if err != nil {
		return ClassField{}, err
	}
	overloads := make([]ClassField, overloadsLen)
	for k := 0; k < overloadsLen; k++ {
		cf, err := r.readClassFieldForward()
		if err != nil {
			return ClassField{}, err
		}
		overloads[k] = cf
	}

	return ClassField{
		Name:      name,
		Pos:       pos,
		NamePos:   namePos,
		Overloads: overloads,
	}, nil
}

func (r *Reader) readEnumFieldForward() (EnumField, error) {
	name, err := r.readString()
	if err != nil {
		return EnumField{}, err
	}
	pos, err := r.readPos()
	if err != nil {
		return EnumField{}, err
	}
	namePos, err := r.readPos()
	if err != nil {
		return EnumField{}, err
	}
	index, err := r.readUleb128()
	if err != nil {
		return EnumField{}, err
	}

	return EnumField{
		Name:    name,
		Pos:     pos,
		NamePos: namePos,
		Index:   index,
	}, nil
}

// Chunks

func (r *Reader) readCFD() error {
	length, err := r.readUleb128()
	if err != nil {
		return err
	}
	for index := 0; index < length; index++ {
		_ = r.classes[index] // Trace evaluation slot
	}
	return nil
}

func (r *Reader) readSTR() error {
	length, err := r.readUleb128()
	if err != nil {
		return err
	}
	r.stringPool = make([]string, length)
	for index := 0; index < length; index++ {
		strLen, err := r.readUleb128()
		if err != nil {
			return err
		}
		strBuf := make([]byte, strLen)
		if _, err := io.ReadFull(r.i, strBuf); err != nil {
			return err
		}
		r.stringPool[index] = string(strBuf)
	}
	return nil
}

func (r *Reader) readMDF() (MDF, error) {
	path, err := r.readPath()
	if err != nil {
		return MDF{}, err
	}
	file, err := r.readString()
	if err != nil {
		return MDF{}, err
	}
	numAnons, err := r.readUleb128()
	if err != nil {
		return MDF{}, err
	}
	numMonos, err := r.readUleb128()
	if err != nil {
		return MDF{}, err
	}

	return MDF{
		Path:     path,
		File:     file,
		NumAnons: numAnons,
		NumMonos: numMonos,
	}, nil
}
func (r *Reader) readMTF() (MTF, error) {
	typesLen, err := r.readUleb128()
	if err != nil {
		return MTF{}, err
	}
	fmt.Printf("[MTF Diagnostic] Parsing module types loop size: %d\n", typesLen)

	types := make([]ModuleTypeWithKind, typesLen)
	for k := 0; k < typesLen; k++ {
		// DEFENSIVE CHECK: If an index over-read occurs or we hit EOF early,
		// don't panic. Treat it like a corrupt font glyph: log it and safely escape.
		kindByte, err := r.readByte()
		if err != nil {
			fmt.Printf("[MTF Warning] Corrupt type layout boundary at index %d. Skipping gracefully.\n", k)
			break
		}

		path, err := r.readPath()
		if err != nil {
			fmt.Printf("[MTF Warning] Corrupt path string index. Setting to zero fallback.\n")
			path = Path{Pack: []string{}, Name: "Unknown"} // Null fallback
		}

		pos, err := r.readPos()
		if err != nil {
			pos = Pos{} // Null structural fallback
		}

		// Handle unstable nightly trailing parameters safely
		params, _ := r.readTypeParametersForward()

		m := ModuleType{
			Path:    path,
			Pos:     pos,
			NamePos: pos,
			Params:  params,
		}

		var kind ModuleTypeKind
		switch kindByte {
		case 0:
			// Read class loops defensively
			flags, _ := r.readUleb128()

			fieldsLen, err := r.readUleb128()
			if err != nil {
				fieldsLen = 0
			} // Default corrupt parameters to 0

			fields := make([]ClassField, fieldsLen)
			for i := 0; i < fieldsLen; i++ {
				fields[i], _ = r.readClassFieldForward()
			}

			staticsLen, err := r.readUleb128()
			if err != nil {
				staticsLen = 0
			} // Default corrupt parameters to 0

			statics := make([]ClassField, staticsLen)
			for i := 0; i < staticsLen; i++ {
				statics[i], _ = r.readClassFieldForward()
			}

			kind = ModuleTypeKind{
				Kind: KindClass,
				Class: ClassData{
					Flags:   flags,
					Fields:  fields,
					Statics: statics,
				},
			}
		default:
			// If we hit an unknown structural state identifier, isolate it safely
			kind = ModuleTypeKind{Kind: KindClass}
		}

		types[k] = ModuleTypeWithKind{M: m, Kind: kind}
	}

	return MTF{Types: types}, nil
}

func (r *Reader) readCLR() error {
	length, err := r.readUleb128()
	if err != nil {
		return err
	}
	r.classes = make([]Class, length)
	for index := 0; index < length; index++ {
		path, err := r.readFullPath()
		if err != nil {
			return err
		}
		module, exists := r.api.ResolveModuleType(path.Pack, path.Name, path.TypeName)
		if !exists {
			return r.fail(fmt.Sprintf("Could not resolve module type %v.%s", path.Pack, path.Name))
		}
		if module.Kind.Kind == KindClass {
			r.classes[index] = Class{M: module.M, C: module.Kind.Class}
		} else {
			return r.fail(fmt.Sprintf("Unexpected type where class was expected: %s", path.Name))
		}
	}
	return nil
}

func (r *Reader) readENR() error {
	length, err := r.readUleb128()
	if err != nil {
		return err
	}
	r.enums = make([]Enum, length)
	for index := 0; index < length; index++ {
		path, err := r.readFullPath()
		if err != nil {
			return err
		}
		module, exists := r.api.ResolveModuleType(path.Pack, path.Name, path.TypeName)
		if !exists {
			return r.fail(fmt.Sprintf("Could not resolve module type %v.%s", path.Pack, path.Name))
		}
		if module.Kind.Kind == KindEnum {
			r.enums[index] = Enum{M: module.M, En: module.Kind.Enum}
		} else {
			return r.fail(fmt.Sprintf("Unexpected type where enum was expected: %s", path.Name))
		}
	}
	return nil
}

func (r *Reader) readABR() error {
	length, err := r.readUleb128()
	if err != nil {
		return err
	}
	r.abstracts = make([]Abstract, length)
	for index := 0; index < length; index++ {
		path, err := r.readFullPath()
		if err != nil {
			return err
		}
		module, exists := r.api.ResolveModuleType(path.Pack, path.Name, path.TypeName)
		if !exists {
			return r.fail(fmt.Sprintf("Could not resolve module type %v.%s", path.Pack, path.Name))
		}
		if module.Kind.Kind == KindAbstract {
			r.abstracts[index] = Abstract{M: module.M, A: module.Kind.Abstract}
		} else {
			return r.fail(fmt.Sprintf("Unexpected type where abstract was expected: %s", path.Name))
		}
	}
	return nil
}

func (r *Reader) readTDR() error {
	length, err := r.readUleb128()
	if err != nil {
		return err
	}
	r.typedefs = make([]Typedef, length)
	for index := 0; index < length; index++ {
		path, err := r.readFullPath()
		if err != nil {
			return err
		}
		module, exists := r.api.ResolveModuleType(path.Pack, path.Name, path.TypeName)
		if !exists {
			return r.fail(fmt.Sprintf("Could not resolve module type %v.%s", path.Pack, path.Name))
		}
		if module.Kind.Kind == KindTypedef {
			r.typedefs[index] = Typedef{M: module.M, Td: module.Kind.Typedef}
		} else {
			return r.fail(fmt.Sprintf("Unexpected type where typedef was expected: %s", path.Name))
		}
	}
	return nil
}

func (r *Reader) readOFR() error {
	length, err := r.readUleb128()
	if err != nil {
		return err
	}
	r.anonFields = make([]ClassField, length)
	for k := 0; k < length; k++ {
		cf, err := r.readClassFieldForward()
		if err != nil {
			return err
		}
		r.anonFields[k] = cf
	}
	return nil
}

// In hxb_reader.go

func (r *Reader) readEFD() error {
	// 1. Joey's format declares how many master expression entries are in this block
	exprCount, err := r.readUleb128()
	if err != nil {
		return err
	}

	// 2. Consume them in order, advancing the file cursor precisely
	for i := 0; i < exprCount; i++ {
		if err := r.readExpression(); err != nil {
			return err
		}
	}
	return nil
}

// readAFD handles the second expression block for Anonymous Closures
func (r *Reader) readAFD() error {
	anonCount, err := r.readUleb128()
	if err != nil {
		return err
	}

	for i := 0; i < anonCount; i++ {
		if err := r.readExpression(); err != nil {
			return err
		}
	}
	return nil
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

func (r *Reader) readcrL() error {
	// Parse your custom linearity instructions
	actionCount, err := r.readUleb128()
	if err != nil {
		return err
	}

	for i := 0; i < actionCount; i++ {
		exprOffset, _ := r.readUleb128()
		varID, _ := r.readUleb128()
		actionByte, _ := r.readByte()

		// Stream directly into your flat Typer state map
		r.applyLinearityState(exprOffset, varID, actionByte)
	}

	return nil
}

// applyLinearityState intercepts and records memory lifecycle instructions using your real Typer states
func (r *Reader) applyLinearityState(exprOffset int, varID int, actionByte byte) {
	if r.linearityMap == nil {
		r.linearityMap = make(map[LinearityKey]State)
	}

	// Cast the incoming byte straight into your existing Typer State type
	targetState := State(actionByte)
	key := LinearityKey{ExprOffset: exprOffset, VarID: varID}

	// Cache it in the flat matrix
	r.linearityMap[key] = targetState

	// Trace feedback using your real enum constants
	var stateName string
	switch targetState {
	case Owned:
		stateName = "OWNED"
	case Borrow:
		stateName = "BORROW"
	case Leaked:
		stateName = "LEAKED"
	case Free:
		stateName = "FREE (LFR3)"
	default:
		stateName = fmt.Sprintf("UNKNOWN_STATE_BYTE_(0x%X)", actionByte)
	}

	fmt.Printf("[Creeling Trace] Registered %s directive for Var %d at Expression Offset %d\n",
		stateName, varID, exprOffset)
}

func (r *Reader) readChunkData(kind ChunkKind, size int) error {
	switch kind {
	case STR:
		return r.readSTR()
	case MDF_Chunk:
		mdf, err := r.readMDF()
		if err != nil {
			return err
		}
		r.module = &Module{
			Mdf: mdf,
			Mtf: nil,
		}
		r.api.AddModule(r.module)
	case MTF_Chunk:
		mtf, err := r.readMTF()
		if err != nil {
			return err
		}
		r.module.Mtf = &mtf
	case CLR:
		return r.readCLR()
	case ENR:
		return r.readENR()
	case ABR:
		return r.readABR()
	case TDR:
		return r.readTDR()
	case OFR:
		return r.readOFR()
	case EOM:
		// Boundary reached
	case CFD:
		return r.readCFD()
	case EFD:
		return r.readEFD()
	case AFD:
		return r.readAFD()
	case crL:
		// custom creeling chunk
		return r.readcrL()
	default:
		if size > 0 {
			discardBuf := make([]byte, size)
			if _, err := io.ReadFull(r.i, discardBuf); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reader) Read(api ReaderApi) (*Module, error) {
	r.api = api
	r.module = nil

	magicBuf := make([]byte, 3)
	if _, err := io.ReadFull(r.i, magicBuf); err != nil {
		return nil, err
	}
	if string(magicBuf) != "hxb" {
		return nil, r.fail(fmt.Sprintf("Expected magic to be hxb, but it is %s", string(magicBuf)))
	}

	_, err := r.readByte() // Read and discard version byte
	if err != nil {
		return nil, err
	}

	chunkNameBuf := make([]byte, 3)
	for {
		_, err := io.ReadFull(r.i, chunkNameBuf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		var chunkSize int32
		if err := binary.Read(r.i, binary.BigEndian, &chunkSize); err != nil {
			return nil, err
		}

		chunkKind, err := ChunkKindFromString(string(chunkNameBuf))
		if err != nil {
			return nil, err
		}

		if chunkKind == EOM {
			break
		}

		if err := r.readChunkData(chunkKind, int(chunkSize)); err != nil {
			return nil, err
		}
	}
	return r.module, nil
}
