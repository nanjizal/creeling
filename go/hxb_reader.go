package main

import (
	"encoding/binary"
	"fmt"
	"io"
)

type ReaderApi interface {
	ResolveModuleType(pack []string, name string, typeName string) (ModuleTypeWithKind, bool)
	AddModule(module *Module)
}

type LinearityKey struct{ ExprOffset, VarID int }

type Reader struct {
	i                 io.Reader
	api               ReaderApi
	stringPool        []string
	classes           []Class
	enums             []Enum
	typedefs          []Typedef
	abstracts         []Abstract
	anonFields        []ClassField
	module            *Module
	linearityMap      map[LinearityKey]State
	currentExprOffset int
	Nodes             []Node
	err               error
}

func NewReader(i io.Reader) *Reader {
	return &Reader{i: i, linearityMap: make(map[LinearityKey]State), Nodes: make([]Node, 0)}
}

func (r *Reader) fail(res string) error { return fmt.Errorf("%w: %s", ErrHxbReaderException, res) }

// --- Optimized Functional Primitives ---

func (r *Reader) checkByte() byte {
	if r.err != nil {
		return 0
	}
	var b [1]byte
	if _, err := r.i.Read(b[:]); err != nil {
		r.err = err
	}
	return b[0]
}

func (r *Reader) checkVarint() int {
	if r.err != nil {
		return 0
	}
	res, shift := 0, 0
	for {
		b := r.checkByte()
		if r.err != nil {
			return 0
		}
		res |= int(b&0x7F) << shift
		if b < 0x80 {
			break
		}
		shift += 7
	}
	return res
}

func (r *Reader) checkSignedVarint() int {
	if r.err != nil {
		return 0
	}
	res, shift := 0, 0
	var b byte
	for {
		b = r.checkByte()
		if r.err != nil {
			return 0
		}
		res |= int(b&0x7F) << shift
		shift += 7
		if b < 0x80 {
			break
		}
	}
	if (b & 0x40) != 0 {
		res |= ^0 << shift
	}
	return res
}

func (r *Reader) checkString() string {
	idx := r.checkVarint()
	if r.err != nil || idx < 0 || idx >= len(r.stringPool) {
		r.err = r.fail(fmt.Sprintf("String pool out of bounds: %d", idx))
		return ""
	}
	return r.stringPool[idx]
}

func (r *Reader) checkExpr() Node {
	if r.err != nil {
		return Node{}
	}
	node, _ := r.readExpression()
	return node
}

// --- Haxe-Style Higher-Order Structural Readers ---

func readList[T any](r *Reader, parser func() T) []T {
	length := r.checkVarint()
	if r.err != nil {
		return nil
	}
	list := make([]T, length)
	for i := 0; i < length; i++ {
		list[i] = parser()
	}
	return list
}

func (r *Reader) readPath() Path {
	return Path{Pack: readList(r, r.checkString), Name: r.checkString()}
}

func (r *Reader) readFullPath() FullPath {
	return FullPath{Path: r.readPath(), TypeName: r.checkString()}
}

func (r *Reader) readPos() Pos {
	return Pos{File: r.checkString(), Min: r.checkVarint(), Max: r.checkVarint()}
}

func (r *Reader) readTypeParameterHost() TypeParameterHost {
	b := r.checkByte()
	if b > 5 {
		r.err = r.fail(fmt.Sprintf("Unknown host byte: %d", b))
		return 0
	}
	return TypeParameterHost(b)
}

func (r *Reader) readTypeParametersForward() []TypeParameter {
	return readList(r, func() TypeParameter {
		return TypeParameter{Path: r.readPath(), Pos: r.readPos(), Host: r.readTypeParameterHost()}
	})
}

// --- Fields & Compound Elements ---

func (r *Reader) readClassFieldForward() ClassField {
	return ClassField{
		Name: r.checkString(), Pos: r.readPos(), NamePos: r.readPos(),
		Overloads: readList(r, r.readClassFieldForward),
	}
}

func (r *Reader) readEnumFieldForward() EnumField {
	return EnumField{Name: r.checkString(), Pos: r.readPos(), NamePos: r.readPos(), Index: r.checkVarint()}
}

// --- Chunk Context Decoding Passes ---

func (r *Reader) readCFD() {
	readList(r, func() int { return r.checkVarint() }) // Dumps references smoothly
}

func (r *Reader) readSTR() error {
	r.stringPool = readList(r, func() string {
		strLen := r.checkVarint()
		if r.err != nil {
			return ""
		}
		buf := make([]byte, strLen)
		if _, err := io.ReadFull(r.i, buf); err != nil {
			r.err = err
			return ""
		}
		return string(buf)
	})
	return r.err
}

func (r *Reader) readMDF() (MDF, error) {
	return MDF{Path: r.readPath(), File: r.checkString(), NumAnons: r.checkVarint(), NumMonos: r.checkVarint()}, r.err
}

func (r *Reader) readMTF() (MTF, error) {
	types := readList(r, func() ModuleTypeWithKind {
		kindByte := r.checkByte()
		m := ModuleType{Path: r.readPath(), Pos: r.readPos(), NamePos: r.readPos(), Params: r.readTypeParametersForward()}
		var kind ModuleTypeKind
		if kindByte == 0 {
			kind = ModuleTypeKind{Kind: KindClass, Class: ClassData{
				Flags:   r.checkVarint(),
				Fields:  readList(r, r.readClassFieldForward),
				Statics: readList(r, r.readClassFieldForward),
			}}
		} else {
			kind = ModuleTypeKind{Kind: ModuleTypeKindType(kindByte)}
		}
		return ModuleTypeWithKind{M: m, Kind: kind}
	})
	return MTF{Types: types}, r.err
}

func (r *Reader) readCLR() error {
	readList(r, func() int {
		path := r.readFullPath()
		if r.err != nil {
			return 0
		}
		m, ok := r.api.ResolveModuleType(path.Pack, path.Name, path.TypeName)
		if !ok || m.Kind.Kind != KindClass {
			r.err = r.fail("Class resolution mismatch")
			return 0
		}
		r.classes = append(r.classes, Class{M: m.M, C: m.Kind.Class})
		return 0
	})
	return r.err
}

func (r *Reader) readENR() error {
	readList(r, func() int {
		path := r.readFullPath()
		if r.err != nil {
			return 0
		}
		m, ok := r.api.ResolveModuleType(path.Pack, path.Name, path.TypeName)
		if !ok || m.Kind.Kind != KindEnum {
			r.err = r.fail("Enum resolution mismatch")
			return 0
		}
		r.enums = append(r.enums, Enum{M: m.M, En: m.Kind.Enum})
		return 0
	})
	return r.err
}

func (r *Reader) readABR() error {
	readList(r, func() int {
		path := r.readFullPath()
		if r.err != nil {
			return 0
		}
		m, ok := r.api.ResolveModuleType(path.Pack, path.Name, path.TypeName)
		if !ok || m.Kind.Kind != KindAbstract {
			r.err = r.fail("Abstract resolution mismatch")
			return 0
		}
		r.abstracts = append(r.abstracts, Abstract{M: m.M, A: m.Kind.Abstract})
		return 0
	})
	return r.err
}

func (r *Reader) readTDR() error {
	readList(r, func() int {
		path := r.readFullPath()
		if r.err != nil {
			return 0
		}
		m, ok := r.api.ResolveModuleType(path.Pack, path.Name, path.TypeName)
		if !ok || m.Kind.Kind != KindTypedef {
			r.err = r.fail("Typedef resolution mismatch")
			return 0
		}
		r.typedefs = append(r.typedefs, Typedef{M: m.M, Td: m.Kind.Typedef})
		return 0
	})
	return r.err
}

func (r *Reader) readOFR() error { r.anonFields = readList(r, r.readClassFieldForward); return r.err }
func (r *Reader) readEFD() error {
	readList(r, func() int { r.Nodes = append(r.Nodes, r.checkExpr()); return 0 })
	return r.err
}
func (r *Reader) readAFD() error {
	readList(r, func() int { r.Nodes = append(r.Nodes, r.checkExpr()); return 0 })
	return r.err
}

func (r *Reader) readcrL() error {
	readList(r, func() int {
		eo, id, b := r.checkVarint(), r.checkVarint(), r.checkByte()
		r.checkSignedVarint()
		if r.err == nil {
			r.applyLinearityState(eo, id, b)
		}
		return 0
	})
	return r.err
}

func (r *Reader) applyLinearityState(eo, id int, b byte) {
	if r.linearityMap == nil {
		r.linearityMap = make(map[LinearityKey]State)
	}
	r.linearityMap[LinearityKey{ExprOffset: eo, VarID: id}] = State(b)
}

// --- The High-Speed Static Jump Router ---

func (r *Reader) readChunkData(kind ChunkKind, size int) error {
	switch kind {
	case STR:
		return r.readSTR()
	case MDF_Chunk:
		if mdf, err := r.readMDF(); err == nil {
			r.module = &Module{Mdf: mdf}
			r.api.AddModule(r.module)
		}
	case MTF_Chunk:
		if mtf, err := r.readMTF(); err == nil && r.module != nil {
			r.module.Mtf = &mtf
		}
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
	case CFD:
		r.readCFD()
	case EFD:
		return r.readEFD()
	case AFD:
		return r.readAFD()
	case EXD:
		r.Nodes = append(r.Nodes, r.checkExpr())
	case crL:
		return r.readcrL()
	default:
		if size > 0 && r.err == nil {
			buf := make([]byte, size)
			if _, err := io.ReadFull(r.i, buf); err != nil {
				r.err = err
			}
		}
	}
	return r.err
}

// --- Main Driving Stream File Parser ---

func (r *Reader) Read(api ReaderApi) (*Module, error) {
	r.api = api
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r.i, magic); err != nil || string(magic) != "hxb\x01" {
		return nil, r.fail("Invalid magic header")
	}

	window := make([]byte, 3)
	for {
		if _, err := io.ReadFull(r.i, window); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		nameStr := string(window)
		kind, err := ChunkKindFromString(nameStr)

		// Self-healing Fixed-Window Anchor Scanner Loop
		if err != nil || !validChunks[nameStr] {
			for {
				var b [1]byte
				if _, err := io.ReadFull(r.i, b[:]); err != nil {
					return nil, err
				}
				window[0], window[1], window[2] = window[1], window[2], b[0]
				nameStr = string(window)
				if validChunks[nameStr] {
					kind, _ = ChunkKindFromString(nameStr)
					break
				}
			}
		}

		var size int32
		if err := binary.Read(r.i, binary.BigEndian, &size); err != nil {
			return nil, err
		}
		if kind == EOM {
			break
		}

		r.err = nil
		if kind == EXD || kind == STR || kind == MDF_Chunk || kind == EFD || kind == crL {
			r.readChunkData(kind, int(size))
		} else if size > 0 && size < 1000000 {
			buf := make([]byte, size)
			io.ReadFull(r.i, buf)
		}
	}
	return r.module, nil
}
