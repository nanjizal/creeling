package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// 1. Implement a clean MockApi to satisfy the ReaderApi interface
type MockApi2 struct{}

// ResolveModuleType is called by the reader when resolving external dependencies (like Class/Enum lookups)
func (m *MockApi2) ResolveModuleType(pack []string, name string, typeName string) (ModuleTypeWithKind, bool) {
	fmt.Printf("[Test API] Resolving type dependency lookup: %v.%s (%s)\n", pack, name, typeName)
	// Return a default blank template structure for isolated parser checking
	return ModuleTypeWithKind{}, true
}

// AddModule is called when the reader successfully initializes an MDF (Module Definition) chunk
func (m *MockApi2) AddModule(module *Module) {
	fmt.Printf("[Test API] Registering Module Definition: %s (File Target: %s)\n",
		module.Mdf.Path.Name, module.Mdf.File)
}

// 2. The Complete Verification Test
func TestExportHxbToJson(t *testing.T) {
	// Open the generated compiler binary stream target
	// Make sure you place your compiled 'test.pkg.Test.hxb' file in this exact folder!
	file, err := os.Open("test.pkg.Test.hxb")
	if err != nil {
		t.Fatalf("Failed to locate hxb test target file: %v. Make sure to generate it via the Haxe compiler first!", err)
	}
	defer file.Close()

	// Initialize your ported Go reader framework
	reader := NewReader(file)
	api := &MockApi2{}

	// Trigger the main binary parsing loop execution pass
	module, err := reader.Read(api)
	if err != nil {
		t.Fatalf("Ported Go HxbReader crashed during binary block decoding: %v", err)
	}

	// Double check that we actually extracted data nodes
	if module == nil {
		t.Fatal("Reader executed but returned an empty (nil) module structure mapping")
	}

	// 3. Marshall the complete Module struct data layout into beautifully indented JSON bytes
	jsonBytes, err := json.MarshalIndent(module, "", "  ")
	if err != nil {
		t.Fatalf("Failed to serialize module structures to JSON payload strings: %v", err)
	}

	// 4. Output the finalized JSON string file straight back onto your disk
	err = os.WriteFile("test_module.json", jsonBytes, 0644)
	if err != nil {
		t.Fatalf("Failed to save JSON data output file: %v", err)
	}

	fmt.Println("\n🎉 Success! 'test_module.json' has been completely generated in your project root folder.")
}
