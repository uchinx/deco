# deco

A CLI tool that finds dead code in Go projects. It reports exported struct methods and interface methods that are never called anywhere in the module.

## Install

```
go install github.com/uchinx/deco/cmd/deco@latest
```

Or build from source:

```
git clone https://github.com/uchinx/deco.git
cd deco
go build ./cmd/deco
```

## Usage

```
# Scan all packages in current module
deco ./...

# Scan a specific package
deco ./pkg/mylib

# Scan multiple packages
deco ./internal/auth ./internal/db
```

### Output

```
path/to/file.go:42	*MyStruct.UnusedMethod	(struct_method)
path/to/file.go:88	MyInterface.NeverCalled	(interface_method)
```

Each line shows the file position, the type and method name, and whether it's a struct method or interface method.

### Exit codes

- `0` — no dead code found
- `1` — dead code found
- `2` — error (e.g. package load failure)

## How it works

deco uses `golang.org/x/tools/go/packages` with full type checking to:

1. **Collect declarations** — all exported methods on exported struct and interface types
2. **Collect call sites** — every method call across all packages (including tests), resolved to exact `types.Func` identity
3. **Compare** — any declared method not in the call set is dead code

### What it skips

- Unexported methods and methods on unexported types
- `main()` and `init()` functions
- Methods satisfying common stdlib interfaces: `error`, `fmt.Stringer`, `json.Marshaler`, `json.Unmarshaler`
- Test file declarations (but test file _calls_ count as usage)

## Comparison with other tools

| Feature | deco | staticcheck (U1000) | golang.org/x/tools/cmd/deadcode |
|---|---|---|---|
| Detects unused struct methods | Yes | Yes | Yes |
| Detects unused interface methods | Yes | No | No |
| Cross-references interface/concrete implementations | Yes | No | No |
| Ignores mock usage in test files | Yes | No | N/A |
| `//nolint:unused` directive support | Yes | No (uses own directives) | No |
| Reports unexported type methods | Yes | Yes | Yes |
| Whole-program analysis | Module-level | Package-level | Module-level |
| Stdlib interface detection (`error`, `Stringer`, etc.) | Yes | Yes | Yes |

### Key differences

**vs staticcheck (U1000)**

staticcheck reports unused code at the package level. It catches unexported functions, types, and methods, but does not analyze whether an _interface method_ is unused across the module. If a method is declared in an interface, staticcheck considers it "used" by the interface definition itself. deco treats interface methods as unused if no code ever calls them.

**vs deadcode (`golang.org/x/tools/cmd/deadcode`)**

deadcode focuses on unreachable functions using call graph analysis (starting from `main`). It does not analyze interface method declarations — it only tracks concrete function reachability. deco specifically targets the gap: exported methods (on both structs and interfaces) that are declared but never invoked anywhere in the module.

**When to use deco**

- You have large interfaces and want to find methods that no one calls
- You want to clean up concrete methods left behind after removing interface methods
- You want mock-aware analysis that won't count test mock usage as "real" usage

## License

MIT
