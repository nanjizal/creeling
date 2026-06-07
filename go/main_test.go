package main

import (
	"fmt"
	"os"
	"testing"
)

// MockApi implements the ReaderApi interface for our test pass
type MockApi struct{}

func (m *MockApi) ResolveModuleType(pack []string, name string, typeName string) (ModuleTypeWithKind, bool) {
	fmt.Printf("[Test API] Resolving type lookup: %v.%s (%s)\n", pack, name, typeName)
	// Return a blank template stub for isolation testing
	return ModuleTypeWithKind{}, true
}

func (m *MockApi) AddModule(module *Module) {
	fmt.Printf("[Test API] Successfully registered Module: %s (File: %s)\n",
		module.Mdf.Path.Name, module.Mdf.File)
}

func TestHxbReaderRegression(t *testing.T) {
	// 1. Open your generated compiler binary
	file, err := os.Open("test.pkg.Test.hxb")
	if err != nil {
		t.Fatalf("Failed to locate hxb test target file: %v. Make sure to generate it via Haxe first!", err)
	}
	defer file.Close()

	// 2. Initialize the ported Go reader
	reader := NewReader(file)
	api := &MockApi{}

	// 3. Trigger the main execution loop
	module, err := reader.Read(api)
	if err != nil {
		t.Fatalf("Ported HxbReader crashed during binary decoding: %v", err)
	}

	// 4. Run structural validation assertions
	if module == nil {
		t.Fatal("Reader returned a nil module structure mapping")
	}

	fmt.Println("--- String Pool Extracted ---")
	for idx, str := range reader.stringPool {
		fmt.Printf(" [%d] -> %s\n", idx, str)
	}

	if len(reader.stringPool) == 0 {
		t.Error("Validation Failed: Extracted string pool chunk is empty")
	}
}
