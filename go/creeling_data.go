package main

// creeling_data.go

// === 1. Lifecycle & Hardware States ===

type State int

const (
	Owned  State = 0
	Borrow State = 1
	Leaked State = 2
	Free   State = 3
)

type MemoryTarget byte

const (
	Managed       MemoryTarget = 0x00 // Standard target GC heap
	Stack         MemoryTarget = 0x01 // CPU Local frame allocation
	UnmanagedHeap MemoryTarget = 0x02 // Manual malloc/free allocations
	Inlined       MemoryTarget = 0x03 // Flattened directly inside parent container
)

// === 2. Opcode Definitions ===

type FullOpcode byte

const (
	Eof          FullOpcode = 0
	EConst       FullOpcode = 1
	ELocal       FullOpcode = 2
	EArray       FullOpcode = 3
	EBinop       FullOpcode = 4
	EField       FullOpcode = 5
	EType        FullOpcode = 6
	EParenthesis FullOpcode = 7
	EObjectDecl  FullOpcode = 8
	EArrayDecl   FullOpcode = 9
	ECall        FullOpcode = 10
	ENew         FullOpcode = 11
	EUnop        FullOpcode = 12
	EFunction    FullOpcode = 13
	EBlock       FullOpcode = 14
	EFor         FullOpcode = 15
	EIn          FullOpcode = 16
	EIf          FullOpcode = 17
	EWhile       FullOpcode = 18
	ESwitch      FullOpcode = 19
	ETry         FullOpcode = 20
	EReturn      FullOpcode = 21
	EBreak       FullOpcode = 22
	EContinue    FullOpcode = 23
	EUntyped     FullOpcode = 24
	EThrow       FullOpcode = 25
	ECast        FullOpcode = 26
	EDisplay     FullOpcode = 27
	ETernary     FullOpcode = 28
	ECheckType   FullOpcode = 29
	EMeta        FullOpcode = 30
)

// === 3. Unified Shared Structures ===

// Node represents an expression element inside the AST stream.
// Shared by: Reader/Annotator (to parse) and Typer (to analyze flow).
type Node struct {
	Kind      FullOpcode
	VarID     int
	MethodID  string
	Field     string
	Offset    int // The byte offset where this node begins in the binary
	Nodes     []Node
	ThenBlock []Node
	ElseBlock []Node
}

// Track unifies behavior (Flow), hardware target (Place), type, and offset layout constraints.
// Shared by: Reader/Annotator (to define variables) and Typer/Writer (to generate code outputs).
type Track struct {
	ID    int          `json:"id"`
	Flow  State        `json:"flow"`  // lifecycle state
	Place MemoryTarget `json:"place"` // hardware location (Managed, Stack, etc.)
	Type  string       `json:"type"`  // target language data type string
	Off   int32        `json:"off"`   // byte layout displacement offset size
}

// Tag stores the resolved (position, varID, state) triple that Pass2 writes into the crL chunk.
// Shared by: Typer (to output lifecycle results) and Annotator (to write binary chunks).
type Tag struct {
	Pos   int   // absolute file byte index / expression offset carried through from Node.Offset
	VarID int   // target variable id
	Place byte  // byte written into the crL chunk (interpreted as flow State on read-back)
	Off   int32 // reserved for a future hardware-layout pass
}

// Flow tracks a variable's transient data-flow lifecycle during analysis passes.
type Flow struct {
	ID     int   // Target variable identifier
	State  State // Active lifecycle status (Owned, Borrow, Leaked, Free)
	Offset int   // The expression bytecode offset marker where this tracking state was captured
}
