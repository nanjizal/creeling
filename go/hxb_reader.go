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
package main

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

type Reader struct {
	i          io.Reader
	api        ReaderApi
	stringPool []string
	classes    []Class
	enums      []Enum
	typedefs   []Typedef
	abstracts  []Abstract
	anonFields []ClassField
	module     *Module
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
	types := make([]ModuleTypeWithKind, typesLen)

	for k := 0; k < typesLen; k++ {
		kindByte, err := r.readByte()
		if err != nil {
			return MTF{}, err
		}

		path, err := r.readPath()
		if err != nil {
			return MTF{}, err
		}
		pos, err := r.readPos()
		if err != nil {
			return MTF{}, err
		}
		namePos, err := r.readPos()
		if err != nil {
			return MTF{}, err
		}
		params, err := r.readTypeParametersForward()
		if err != nil {
			return MTF{}, err
		}

		m := ModuleType{
			Path:    path,
			Pos:     pos,
			NamePos: namePos,
			Params:  params,
		}

		var kind ModuleTypeKind
		switch kindByte {
		case 0:
			flags, _ := r.readUleb128()
			var constructor OptionClassField
			if hasConst, _ := r.readOptionMarker(); hasConst {
				cf, _ := r.readClassFieldForward()
				constructor = OptionClassField{HasValue: true, Value: cf}
			}
			var init OptionClassField
			if hasInit, _ := r.readOptionMarker(); hasInit {
				cf, _ := r.readClassFieldForward()
				init = OptionClassField{HasValue: true, Value: cf}
			}

			fieldsLen, _ := r.readUleb128()
			fields := make([]ClassField, fieldsLen)
			for i := 0; i < fieldsLen; i++ {
				fields[i], _ = r.readClassFieldForward()
			}

			staticsLen, _ := r.readUleb128()
			statics := make([]ClassField, staticsLen)
			for i := 0; i < staticsLen; i++ {
				statics[i], _ = r.readClassFieldForward()
			}

			kind = ModuleTypeKind{
				Kind: KindClass,
				Class: ClassData{
					Flags:       flags,
					Constructor: constructor,
					Fields:      fields,
					Statics:     statics,
					Init:        init,
				},
			}
		case 1:
			fieldsLen, _ := r.readUleb128()
			fields := make([]EnumField, fieldsLen)
			for i := 0; i < fieldsLen; i++ {
				fields[i], _ = r.readEnumFieldForward()
			}
			kind = ModuleTypeKind{Kind: KindEnum, Enum: EnumData{Fields: fields}}
		case 2:
			kind = ModuleTypeKind{Kind: KindTypedef, Typedef: TypedefData{}}
		case 3:
			kind = ModuleTypeKind{Kind: KindAbstract, Abstract: AbstractData{}}
		default:
			return MTF{}, r.fail(fmt.Sprintf("Unknown module type byte: %d", kindByte))
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
