package analyzer

// MethodInfo describes a declared method in the codebase.
type MethodInfo struct {
	Name         string // method name
	ReceiverType string // e.g. "MyStruct" or "*MyStruct"
	Package      string // package path
	Position     string // file:line
	Kind         string // "struct_method" or "interface_method"
}

// Result holds the output of a dead code analysis.
type Result struct {
	UnusedMethods []MethodInfo
}
