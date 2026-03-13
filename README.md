# deco

A CLI tool that finds dead code in Go projects. It reports exported struct methods and interface methods that are never called anywhere in the module.

## Install

```
go install github.com/uchin/deco/cmd/deco@latest
```

Or build from source:

```
git clone https://github.com/uchin/deco.git
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
- Test file declarations (but test file *calls* count as usage)

## License

MIT
