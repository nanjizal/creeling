package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Implement the strict HxbStructureParser contract using your real project signatures
type LiveTestApi struct{}

func (m *LiveTestApi) ResolveModuleType(pack []string, name string, typeName string) (ModuleTypeWithKind, bool) {
	return ModuleTypeWithKind{}, true
}

func (m *LiveTestApi) AddModule(module *Module) {}

func main() {
	// A. Align file path targets to match your actual extracted folder hierarchy
	inputFile := "../baseTest/cross/Main.hxb"
	outputFile := "../baseTest/cross/Main.hxbPlus"
	jsonFile := "../baseTest/cross/Main_hxbPlus.json"

	// B. Read the authentic Haxe 5 nightly binary payload bytes
	inputHxbBytes, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Printf("🔴 Integration Error: Cannot locate file '%s'. Check folder depths.\n", inputFile)
		return
	}

	fmt.Printf("--- Step 1: Parsing Authentic Haxe 5 Binary Byte Stream ---\n")
	fmt.Printf("Source Binary Stream Loaded: %d bytes\n", len(inputHxbBytes))

	// C. Instantiate using your clean composition by reference
	myReader := NewReader(bytes.NewReader(inputHxbBytes))
	myTyper := NewTyper()
	annot := NewAnnotator(inputHxbBytes, myReader, myTyper)

	// D. Pass 1: Parse the raw compiler tree structure into nodes using your actual API signature
	/*liveApi := &LiveTestApi{}
	module, err := annot.Pass1(liveApi)
	if err != nil {
		fmt.Printf("🔴 Pass 1 Crash: Decoding error parsing Haxe nightly spec: %v\n", err)
		return
	}
	fmt.Println("✔ Pass 1 Node Parsing completed with zero format errors.")

	*/
	// Pass 1: Parse the raw compiler tree structure into nodes using your actual API signature
	liveApi := &LiveTestApi{}
	module, err := annot.Pass1(liveApi)
	if err != nil {
		fmt.Printf("🔴 Pass 1 Crash: Decoding error parsing Haxe nightly spec: %v\n", err)

		// FIXED DIAGNOSTIC HOOK: Scan the buffer directly for the failing chunk header
		crashIndex := bytes.Index(inputHxbBytes, []byte("XD"))
		if crashIndex == -1 {
			crashIndex = bytes.Index(inputHxbBytes, []byte("EXD"))
		}

		if crashIndex != -1 {
			fmt.Printf("\n--- Binary Stream Diagnosis (Failing Section Index: %d) ---\n", crashIndex)

			startDump := crashIndex - 8
			if startDump < 0 {
				startDump = 0
			}
			endDump := crashIndex + 16
			if endDump > len(inputHxbBytes) {
				endDump = len(inputHxbBytes)
			}

			fmt.Printf("Bytes (Hex):   %x\n", inputHxbBytes[startDump:endDump])
			fmt.Printf("Bytes (ASCII): %s\n", string(inputHxbBytes[startDump:endDump]))
		} else {
			fmt.Println("Could not find 'XD' or 'EXD' marker sequences in the raw byte buffer.")
		}
		return
	}
	// E. Simulate a metadata tag registration for unmanaged tracking layout verification
	annot.Tags = append(annot.Tags, Tag{
		Pos:   len(inputHxbBytes) / 2, // Marks center point for bitstream verification testing
		Place: Stack,
		Off:   -32,
	})

	// F. Pass 2: Execute your byte splicing and hidden crL chunk injection
	hxbPlusBytes := annot.Pass2()
	fmt.Printf("\n--- Step 2: Serializing Generated hxbPlus Architecture ---\n")
	fmt.Printf("Annotated hxbPlus Output Byte Stream: %d bytes\n", len(hxbPlusBytes))

	// G. Write out your new modified hxbPlus format container to disk
	err = os.WriteFile(outputFile, hxbPlusBytes, 0644)
	if err != nil {
		fmt.Printf("🔴 Serialization Error: Cannot cache hxbPlus file: %v\n", err)
		return
	}
	fmt.Printf("✔ Successfully serialized custom target to: %s\n", outputFile)

	// H. Step 3: Serialize the entire parsed Module node tree into indented JSON strings
	fmt.Printf("\n--- Step 3: Marshaling Module Nodes to JSON File ---\n")
	jsonBytes, err := json.MarshalIndent(module, "", "  ")
	if err != nil {
		fmt.Printf("🔴 JSON Serialization Failure: %v\n", err)
		return
	}

	err = os.WriteFile(jsonFile, jsonBytes, 0644)
	if err != nil {
		fmt.Printf("🔴 JSON File Save Failure: %v\n", err)
		return
	}
	fmt.Printf("✔ Success! Visual verification disassembly saved to: %s\n", jsonFile)
}
