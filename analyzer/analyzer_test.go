package analyzer

import (
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func testdataDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "testdata", "src", "example")
}

func TestAnalyze(t *testing.T) {
	dir := testdataDir()
	result, err := Analyze([]string{"./..."}, dir)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Sort for deterministic comparison.
	sort.Slice(result.UnusedMethods, func(i, j int) bool {
		return result.UnusedMethods[i].Name < result.UnusedMethods[j].Name
	})

	// Expected dead code:
	// - MyStruct.UnusedMethod (struct_method)
	// - MyStruct.AnotherUnused (struct_method)
	// - MyInterface.NeverCalled (interface_method)
	//
	// NOT expected:
	// - MyStruct.UsedMethod (called in UseThings)
	// - MyStruct.String (satisfies fmt.Stringer)
	// - MyStruct.Error (satisfies error)
	// - MyStruct.unexportedMethod (not exported)
	// - MyInterface.DoSomething (called in UseThings)
	// - implStruct.DoSomething / NeverCalled (unexported receiver type)

	// Use receiver.name as key to handle multiple methods with different receivers.
	type methodKey struct {
		Receiver string
		Name     string
	}
	expectedMethods := map[methodKey]string{
		{"MyStruct", "AnotherUnused"}:      "struct_method",
		{"MyInterface", "NeverCalled"}:      "interface_method",
		{"*MyStruct", "UnusedMethod"}:       "struct_method",
		{"OrderProcessor", "ProcessBulk"}:   "interface_method",
		{"*RealProcessor", "ProcessBulk"}:   "struct_method",
	}

	if len(result.UnusedMethods) != len(expectedMethods) {
		t.Errorf("expected %d unused methods, got %d:", len(expectedMethods), len(result.UnusedMethods))
		for _, m := range result.UnusedMethods {
			t.Errorf("  %s.%s (%s)", m.ReceiverType, m.Name, m.Kind)
		}
		return
	}

	for _, m := range result.UnusedMethods {
		key := methodKey{m.ReceiverType, m.Name}
		expectedKind, ok := expectedMethods[key]
		if !ok {
			t.Errorf("unexpected unused method: %s.%s (%s)", m.ReceiverType, m.Name, m.Kind)
			continue
		}
		if m.Kind != expectedKind {
			t.Errorf("method %s.%s: expected kind %q, got %q", m.ReceiverType, m.Name, expectedKind, m.Kind)
		}
		if m.Position == "" {
			t.Errorf("method %s.%s: position should not be empty", m.ReceiverType, m.Name)
		}
	}
}

func TestAnalyzeNoDeadCode(t *testing.T) {
	// Analyzing stdlib "fmt" should work without crashing.
	result, err := Analyze([]string{"fmt"}, "")
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	_ = result
}
