package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Analyze loads the Go packages at the given patterns and returns unused methods.
// An optional dir can be passed to set the working directory for package loading.
func Analyze(patterns []string, dir string) (*Result, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedDeps,
		Tests: true,
		Dir:   dir,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	// Check for package load errors.
	var errs []string
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			errs = append(errs, e.Error())
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("package errors:\n%s", strings.Join(errs, "\n"))
	}

	// Separate source packages from test packages.
	// Test packages have IDs ending with " [pkgpath.test]" or are the test binary.
	srcPkgs, allPkgs := categorizePkgs(pkgs)

	// Phase 1: collect declared methods from source (non-test) packages.
	declared := collectDeclarations(srcPkgs)

	// Phase 2: collect call sites from ALL packages (including tests).
	called := collectCallSites(allPkgs)

	// Phase 3: cross-reference interface and concrete methods.
	// If a concrete method is called, mark the interface method as used (and vice versa).
	crossReferenceInterfaceMethods(declared, called, allPkgs)

	// Phase 4: compare — find unused methods.
	var unused []MethodInfo
	for obj, info := range declared {
		if called[obj] {
			continue
		}
		unused = append(unused, info)
	}

	return &Result{UnusedMethods: unused}, nil
}

// categorizePkgs splits loaded packages into source packages and all packages.
// Source packages exclude test files for declaration collection,
// but all packages (including tests) are used for call site collection.
func categorizePkgs(pkgs []*packages.Package) (srcPkgs, allPkgs []*packages.Package) {
	seen := make(map[string]bool)
	var walkAll func(pkg *packages.Package)
	walkAll = func(pkg *packages.Package) {
		if seen[pkg.ID] {
			return
		}
		seen[pkg.ID] = true
		allPkgs = append(allPkgs, pkg)
		for _, imp := range pkg.Imports {
			walkAll(imp)
		}
	}
	for _, pkg := range pkgs {
		walkAll(pkg)
	}

	for _, pkg := range allPkgs {
		if !isTestPkg(pkg) {
			srcPkgs = append(srcPkgs, pkg)
		}
	}
	return srcPkgs, allPkgs
}

func isTestPkg(pkg *packages.Package) bool {
	// Test packages have IDs like "pkg [pkg.test]" or "pkg_test [pkg.test]"
	return strings.Contains(pkg.ID, ".test")
}

// collectDeclarations finds all exported struct methods and interface methods
// in the given packages, skipping test files.
func collectDeclarations(pkgs []*packages.Package) map[*types.Func]MethodInfo {
	declared := make(map[*types.Func]MethodInfo)

	for _, pkg := range pkgs {
		if len(pkg.CompiledGoFiles) != len(pkg.Syntax) {
			continue
		}
		for i, file := range pkg.Syntax {
			filename := pkg.CompiledGoFiles[i]
			if strings.HasSuffix(filename, "_test.go") {
				continue
			}

			ast.Inspect(file, func(n ast.Node) bool {
				switch decl := n.(type) {
				case *ast.FuncDecl:
					collectFuncDecl(pkg, file, decl, declared)
				case *ast.GenDecl:
					if decl.Tok == token.TYPE {
						for _, spec := range decl.Specs {
							ts, ok := spec.(*ast.TypeSpec)
							if !ok {
								continue
							}
							iface, ok := ts.Type.(*ast.InterfaceType)
							if !ok {
								continue
							}
							collectInterfaceMethods(pkg, file, ts, iface, declared)
						}
					}
				}
				return true
			})
		}
	}

	return declared
}

func collectFuncDecl(pkg *packages.Package, file *ast.File, decl *ast.FuncDecl, declared map[*types.Func]MethodInfo) {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return
	}
	if !decl.Name.IsExported() {
		return
	}
	name := decl.Name.Name
	if name == "init" || name == "main" {
		return
	}

	// Check for //nolint:unused in doc comments above or inline.
	if hasNolintDirective(decl.Doc) || hasNolintInLine(pkg, file, decl.Pos()) {
		return
	}

	obj := pkg.TypesInfo.Defs[decl.Name]
	if obj == nil {
		return
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return
	}

	if isStdlibInterfaceMethod(fn) {
		return
	}

	recvType := exprToString(decl.Recv.List[0].Type)
	pos := pkg.Fset.Position(decl.Name.Pos())

	declared[fn] = MethodInfo{
		Name:         name,
		ReceiverType: recvType,
		Package:      pkg.PkgPath,
		Position:     fmt.Sprintf("%s:%d", pos.Filename, pos.Line),
		Kind:         "struct_method",
	}
}

func collectInterfaceMethods(pkg *packages.Package, file *ast.File, ts *ast.TypeSpec, iface *ast.InterfaceType, declared map[*types.Func]MethodInfo) {
	if !ts.Name.IsExported() {
		return
	}

	typeObj := pkg.TypesInfo.Defs[ts.Name]
	if typeObj == nil {
		return
	}

	ifaceType, ok := typeObj.Type().Underlying().(*types.Interface)
	if !ok {
		return
	}

	for i := 0; i < ifaceType.NumMethods(); i++ {
		method := ifaceType.Method(i)
		if !method.Exported() {
			continue
		}
		if isStdlibInterfaceMethod(method) {
			continue
		}
		if hasNolintInLine(pkg, file, method.Pos()) {
			continue
		}

		pos := pkg.Fset.Position(method.Pos())
		declared[method] = MethodInfo{
			Name:         method.Name(),
			ReceiverType: ts.Name.Name,
			Package:      pkg.PkgPath,
			Position:     fmt.Sprintf("%s:%d", pos.Filename, pos.Line),
			Kind:         "interface_method",
		}
	}
}

// crossReferenceInterfaceMethods links interface methods with their concrete
// implementations bidirectionally. If either side is called, both are marked used.
// This prevents false positives where:
//   - a method is called on a concrete type but declared in an interface
//   - a method is called through an interface but declared on a concrete struct
func crossReferenceInterfaceMethods(declared map[*types.Func]MethodInfo, called map[*types.Func]bool, pkgs []*packages.Package) {
	// Collect declared interface methods and struct methods separately.
	var ifaceMethods []*types.Func
	var structMethods []*types.Func
	for fn, info := range declared {
		switch info.Kind {
		case "interface_method":
			ifaceMethods = append(ifaceMethods, fn)
		case "struct_method":
			structMethods = append(structMethods, fn)
		}
	}

	// Build a set of all named types from non-test packages only.
	// Types defined in test files (e.g. mocks) should not bridge
	// interface↔concrete usage, as mock implementations don't
	// represent real usage of the method.
	var namedTypes []*types.Named
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil || pkg.Types == nil {
			continue
		}
		if isTestPkg(pkg) {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if tn, ok := obj.(*types.TypeName); ok {
				if named, ok := tn.Type().(*types.Named); ok {
					namedTypes = append(namedTypes, named)
				}
			}
		}
	}

	// Direction 1: interface method uncalled, but concrete implementation is called.
	for _, ifaceFunc := range ifaceMethods {
		if called[ifaceFunc] {
			continue
		}
		recv := ifaceFunc.Type().(*types.Signature).Recv()
		if recv == nil {
			continue
		}
		ifaceType, ok := recv.Type().Underlying().(*types.Interface)
		if !ok {
			continue
		}

		methodName := ifaceFunc.Name()
		for _, named := range namedTypes {
			for _, typ := range []types.Type{named, types.NewPointer(named)} {
				if !types.Implements(typ, ifaceType) {
					continue
				}
				mset := types.NewMethodSet(typ)
				sel := mset.Lookup(named.Obj().Pkg(), methodName)
				if sel == nil {
					continue
				}
				if fn, ok := sel.Obj().(*types.Func); ok && called[fn] {
					called[ifaceFunc] = true
				}
			}
			if called[ifaceFunc] {
				break
			}
		}
	}

	// Direction 2: struct method uncalled, but the interface method it implements is called.
	for _, structFunc := range structMethods {
		if called[structFunc] {
			continue
		}
		sig, ok := structFunc.Type().(*types.Signature)
		if !ok || sig.Recv() == nil {
			continue
		}
		recvType := sig.Recv().Type()
		methodName := structFunc.Name()

		// Check all interfaces: if recvType implements the interface and
		// the interface's method with the same name is called, mark this as used.
		for _, ifaceFunc := range ifaceMethods {
			if ifaceFunc.Name() != methodName {
				continue
			}
			ifaceRecv := ifaceFunc.Type().(*types.Signature).Recv()
			if ifaceRecv == nil {
				continue
			}
			ifaceType, ok := ifaceRecv.Type().Underlying().(*types.Interface)
			if !ok {
				continue
			}
			// Check if the concrete type (or pointer to it) implements this interface.
			if types.Implements(recvType, ifaceType) || types.Implements(types.NewPointer(recvType), ifaceType) {
				if called[ifaceFunc] {
					called[structFunc] = true
					break
				}
			}
		}

		// Also check interfaces not in our declared set (e.g., interfaces from other packages).
		// Only consider non-test packages to avoid mock usage bridging.
		if !called[structFunc] {
			for _, pkg := range pkgs {
				if pkg.TypesInfo == nil || isTestPkg(pkg) {
					continue
				}
				for _, sel := range pkg.TypesInfo.Selections {
					fn, ok := sel.Obj().(*types.Func)
					if !ok || fn.Name() != methodName {
						continue
					}
					// If this is a call through an interface that our struct implements...
					selRecvType := sel.Recv()
					if selRecvType == nil {
						continue
					}
					iface, ok := selRecvType.Underlying().(*types.Interface)
					if !ok {
						continue
					}
					if types.Implements(recvType, iface) || types.Implements(types.NewPointer(recvType), iface) {
						called[structFunc] = true
						break
					}
				}
				if called[structFunc] {
					break
				}
			}
		}
	}
}

// collectCallSites finds all method calls across all packages.
func collectCallSites(pkgs []*packages.Package) map[*types.Func]bool {
	called := make(map[*types.Func]bool)

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		// Collect from Selections (method calls like x.Method())
		for _, sel := range pkg.TypesInfo.Selections {
			if fn, ok := sel.Obj().(*types.Func); ok {
				called[fn] = true
			}
		}

		// Collect from Uses (direct references, e.g. interface method references)
		for _, obj := range pkg.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok {
				called[fn] = true
			}
		}
	}

	return called
}

// isStdlibInterfaceMethod returns true if the method satisfies a common stdlib interface.
func isStdlibInterfaceMethod(fn *types.Func) bool {
	name := fn.Name()
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return false
	}

	switch name {
	case "Error":
		// error interface: Error() string
		if sig.Params().Len() == 0 && sig.Results().Len() == 1 {
			if basic, ok := sig.Results().At(0).Type().(*types.Basic); ok && basic.Kind() == types.String {
				return true
			}
		}
	case "String":
		// fmt.Stringer: String() string
		if sig.Params().Len() == 0 && sig.Results().Len() == 1 {
			if basic, ok := sig.Results().At(0).Type().(*types.Basic); ok && basic.Kind() == types.String {
				return true
			}
		}
	case "MarshalJSON":
		// json.Marshaler: MarshalJSON() ([]byte, error)
		if sig.Params().Len() == 0 && sig.Results().Len() == 2 {
			return true
		}
	case "UnmarshalJSON":
		// json.Unmarshaler: UnmarshalJSON([]byte) error
		if sig.Params().Len() == 1 && sig.Results().Len() == 1 {
			return true
		}
	}

	return false
}

const nolintDirective = "nolint:unused"

// hasNolintDirective checks if a comment group contains //nolint:unused.
func hasNolintDirective(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, comment := range doc.List {
		if containsNolint(comment.Text) {
			return true
		}
	}
	return false
}

// hasNolintInLine checks if there is a //nolint:unused comment on the same line
// as the given position, or on the line immediately above it.
func hasNolintInLine(pkg *packages.Package, file *ast.File, pos token.Pos) bool {
	targetLine := pkg.Fset.Position(pos).Line
	for _, cg := range file.Comments {
		for _, comment := range cg.List {
			commentLine := pkg.Fset.Position(comment.Pos()).Line
			if (commentLine == targetLine || commentLine == targetLine-1) && containsNolint(comment.Text) {
				return true
			}
		}
	}
	return false
}

func containsNolint(text string) bool {
	return strings.Contains(text, nolintDirective)
}

func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.IndexExpr:
		return exprToString(e.X)
	case *ast.IndexListExpr:
		return exprToString(e.X)
	default:
		return fmt.Sprintf("%T", expr)
	}
}
