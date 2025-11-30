// unified-ir-reader reads and decodes __.PKGDEF files from Go .a archives
// and prints their contents in human-readable format.
package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"io"
	"os"
	"strings"

	"github.com/jespino/unified-ir-reader/pkgbits"
)

func main() {
	// Define flags
	limit := flag.Int("limit", 0, "Limit the number of entries shown per section (0 = show all)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <archive.a>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Decodes and displays the contents of __.PKGDEF from a Go archive file\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	archivePath := flag.Arg(0)

	// Read the archive file
	data, err := os.ReadFile(archivePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Extract __.PKGDEF from the archive
	pkgdefData, err := extractPKGDEF(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting __.PKGDEF: %v\n", err)
		os.Exit(1)
	}

	// Extract the unified IR data
	uirData, err := extractUnifiedIR(pkgdefData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting Unified IR: %v\n", err)
		os.Exit(1)
	}

	// Show detailed binary format information
	if err := showDetailedFormat(uirData, *limit); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding detailed format: %v\n", err)
		os.Exit(1)
	}

	// Decode using the official go/types importer
	if err := decodeWithGoTypes(uirData); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding with go/types: %v\n", err)
		os.Exit(1)
	}
}

// extractPKGDEF extracts the __.PKGDEF section from a .a archive
func extractPKGDEF(data []byte) ([]byte, error) {
	// Check for archive magic
	if !bytes.HasPrefix(data, []byte("!<arch>\n")) {
		return nil, fmt.Errorf("not a valid archive file")
	}

	offset := 8 // Skip "!<arch>\n"

	for offset < len(data) {
		// Each archive entry has a 60-byte header
		if offset+60 > len(data) {
			break
		}

		header := data[offset : offset+60]

		// Parse the file name (16 bytes)
		name := strings.TrimSpace(string(header[0:16]))

		// Parse the file size (10 bytes, decimal ASCII)
		sizeStr := strings.TrimSpace(string(header[48:58]))
		var size int
		fmt.Sscanf(sizeStr, "%d", &size)

		offset += 60

		// Check if this is the __.PKGDEF entry
		if name == "__.PKGDEF" {
			if offset+size > len(data) {
				return nil, fmt.Errorf("truncated archive")
			}
			return data[offset : offset+size], nil
		}

		// Move to next entry (entries are 2-byte aligned)
		offset += size
		if size%2 == 1 {
			offset++ // Skip padding byte
		}
	}

	return nil, fmt.Errorf("__.PKGDEF not found in archive")
}

// extractUnifiedIR extracts the Unified IR data from __.PKGDEF content
func extractUnifiedIR(pkgdefData []byte) ([]byte, error) {
	// The format is:
	// \n$$B\n
	// u<unified-ir-data>
	// \n$$\n

	start := bytes.Index(pkgdefData, []byte("\n$$B\n"))
	if start == -1 {
		return nil, fmt.Errorf("could not find export data start marker")
	}
	start += 5 // Skip "\n$$B\n"

	end := bytes.Index(pkgdefData[start:], []byte("\n$$\n"))
	if end == -1 {
		return nil, fmt.Errorf("could not find export data end marker")
	}

	exportData := pkgdefData[start : start+end]

	// Check for 'u' prefix indicating unified IR
	if len(exportData) == 0 || exportData[0] != 'u' {
		return nil, fmt.Errorf("not unified IR format (expected 'u' prefix)")
	}

	// Return the complete export data including the 'u' prefix
	return exportData, nil
}

// showDetailedFormat shows detailed binary format information
func showDetailedFormat(exportData []byte, limit int) error {
	// Skip the 'u' prefix
	decoder := pkgbits.NewPkgDecoder("", string(exportData[1:]))

	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║          Unified IR Binary Format - Detailed View            ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Show format metadata
	fmt.Println("=== Format Metadata ===")
	fmt.Printf("Sync Markers: %v\n", decoder.SyncMarkers())
	fmt.Printf("Total Elements: %d\n", decoder.TotalElems())

	fp := decoder.Fingerprint()
	fmt.Printf("Fingerprint: %s\n", hex.EncodeToString(fp[:]))
	fmt.Println()

	// Show section statistics
	fmt.Println("=== Section Statistics ===")
	sections := []struct {
		kind pkgbits.SectionKind
		name string
	}{
		{pkgbits.SectionString, "String"},
		{pkgbits.SectionMeta, "Meta"},
		{pkgbits.SectionPosBase, "PosBase"},
		{pkgbits.SectionPkg, "Pkg"},
		{pkgbits.SectionName, "Name"},
		{pkgbits.SectionType, "Type"},
		{pkgbits.SectionObj, "Obj"},
		{pkgbits.SectionObjExt, "ObjExt"},
		{pkgbits.SectionObjDict, "ObjDict"},
		{pkgbits.SectionBody, "Body"},
	}

	for _, sec := range sections {
		count := decoder.NumElems(sec.kind)
		fmt.Printf("  %-12s: %4d elements\n", sec.name, count)
	}
	fmt.Println()

	// Show string table
	fmt.Println("=== String Table (Deduplicated Strings) ===")
	stringCount := decoder.NumElems(pkgbits.SectionString)
	if stringCount > 0 {
		fmt.Printf("Total strings: %d\n", stringCount)
		maxShow := stringCount
		if limit > 0 && limit < stringCount {
			maxShow = limit
			fmt.Printf("(showing first %d)\n", maxShow)
		}
		fmt.Println()

		for i := 0; i < maxShow; i++ {
			str := decoder.StringIdx(pkgbits.Index(i))
			if len(str) > 80 {
				str = str[:77] + "..."
			}
			// Escape special characters
			str = strings.ReplaceAll(str, "\n", "\\n")
			str = strings.ReplaceAll(str, "\t", "\\t")
			fmt.Printf("  [%3d] %q\n", i, str)
		}
		if maxShow < stringCount {
			fmt.Printf("  ... and %d more\n", stringCount-maxShow)
		}
	} else {
		fmt.Println("  (empty)")
	}
	fmt.Println()

	// Show position bases (source files)
	fmt.Println("=== Position Bases (Source Files) ===")
	posBaseCount := decoder.NumElems(pkgbits.SectionPosBase)
	if posBaseCount > 0 {
		maxShow := posBaseCount
		if limit > 0 && limit < posBaseCount {
			maxShow = limit
		}
		for i := 0; i < maxShow; i++ {
			r := decoder.NewDecoder(pkgbits.SectionPosBase, pkgbits.Index(i), pkgbits.SyncPosBase)
			filename := r.String()
			isFileBase := r.Bool()
			if isFileBase {
				fmt.Printf("  [%d] %s (file base)\n", i, filename)
			} else {
				// Line base
				fmt.Printf("  [%d] %s (line base)\n", i, filename)
			}
		}
		if maxShow < posBaseCount {
			fmt.Printf("  ... and %d more\n", posBaseCount-maxShow)
		}
	} else {
		fmt.Println("  (none)")
	}
	fmt.Println()

	// Show package table
	fmt.Println("=== Package Table ===")
	pkgCount := decoder.NumElems(pkgbits.SectionPkg)
	if pkgCount > 0 {
		maxShow := pkgCount
		if limit > 0 && limit < pkgCount {
			maxShow = limit
		}
		for i := 0; i < maxShow; i++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("  [%d] (error reading package: %v)\n", i, r)
					}
				}()
				r := decoder.NewDecoder(pkgbits.SectionPkg, pkgbits.Index(i), pkgbits.SyncPkgDef)
				r.Sync(pkgbits.SyncPkg)
				path := r.String()
				name := r.String()
				if path == "" {
					fmt.Printf("  [%d] <self> (name: %s)\n", i, name)
				} else {
					fmt.Printf("  [%d] %s (name: %s)\n", i, path, name)
				}
			}()
		}
		if maxShow < pkgCount {
			fmt.Printf("  ... and %d more\n", pkgCount-maxShow)
		}
	} else {
		fmt.Println("  (none)")
	}
	fmt.Println()

	// Show type table summary
	fmt.Println("=== Type Table ===")
	typeCount := decoder.NumElems(pkgbits.SectionType)
	fmt.Printf("Total types: %d\n", typeCount)
	fmt.Println("(Type details shown in parsed view below)")
	fmt.Println()

	// Show object table
	fmt.Println("=== Object Table Summary ===")
	objCount := decoder.NumElems(pkgbits.SectionObj)
	if objCount > 0 {
		objCounts := make(map[string]int)
		for i := 0; i < objCount; i++ {
			_, _, tag := decoder.PeekObj(pkgbits.Index(i))
			objCounts[objTagName(tag)]++
		}
		fmt.Printf("Total objects: %d\n", objCount)
		for name, count := range objCounts {
			fmt.Printf("  %-10s: %d\n", name, count)
		}
	} else {
		fmt.Println("  (none)")
	}
	fmt.Println()

	// Show private root (function bodies)
	fmt.Println("=== Private Root (Function Bodies & Internal Data) ===")
	r := decoder.NewDecoder(pkgbits.SectionMeta, pkgbits.PrivateRootIdx, pkgbits.SyncPrivate)
	hasInittask := r.Bool()
	fmt.Printf("Has .inittask: %v\n", hasInittask)

	bodyCount := r.Len()
	fmt.Printf("Function bodies: %d\n", bodyCount)
	if bodyCount > 0 {
		fmt.Println()
		maxShow := bodyCount
		if limit > 0 && limit < bodyCount {
			maxShow = limit
		}
		for i := 0; i < bodyCount; i++ {
			pkgPath := r.String()
			symName := r.String()
			bodyIdx := r.Reloc(pkgbits.SectionBody)
			if i < maxShow {
				fmt.Printf("  [%d] %s.%s (body index: %d)\n", i, pkgPath, symName, bodyIdx)
			}
		}
		if maxShow < bodyCount {
			fmt.Printf("  ... and %d more\n", bodyCount-maxShow)
		}
	}
	r.Sync(pkgbits.SyncEOF)
	fmt.Println()

	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	return nil
}

// buildPKGDEF builds the __.PKGDEF content from export data
func buildPKGDEF(exportData []byte) []byte {
	var buf bytes.Buffer
	// Write a minimal package header
	buf.WriteString("go object darwin amd64 go1.23 X:regabireflect,regabiwrappers,coverageredesign\n")
	buf.WriteString("\n$$B\n")
	buf.Write(exportData)
	buf.WriteString("\n$$\n")
	return buf.Bytes()
}

// writeArchiveEntry writes an archive entry with proper formatting
func writeArchiveEntry(w *bytes.Buffer, name string, content []byte) {
	// Archive entry header is 60 bytes:
	// 0-15:   File name (padded with spaces)
	// 16-27:  File modification timestamp (decimal)
	// 28-33:  Owner ID (decimal)
	// 34-39:  Group ID (decimal)
	// 40-47:  File mode (octal)
	// 48-57:  File size (decimal)
	// 58-59:  Ending characters (`\n)

	header := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8s%-10d`\n",
		name, 0, 0, 0, "644", len(content))

	w.WriteString(header)
	w.Write(content)

	// Archive entries are 2-byte aligned
	if len(content)%2 == 1 {
		w.WriteByte('\n')
	}
}

// decodeWithGoTypes uses the official go/types importer to decode the package
func decodeWithGoTypes(exportData []byte) error {
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║               Package Export Data (Parsed View)               ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// The importer expects a complete package archive file format
	// Build a minimal archive file with just the export data
	var buf bytes.Buffer

	// Write archive header
	buf.WriteString("!<arch>\n")

	// Write __.PKGDEF entry
	pkgdefContent := buildPKGDEF(exportData)
	writeArchiveEntry(&buf, "__.PKGDEF", pkgdefContent)

	// Create a temporary package to import
	fset := token.NewFileSet()

	// Use a custom import function that provides our data
	lookup := func(path string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
	}

	imp := importer.ForCompiler(fset, "gc", lookup)

	// Import the package
	pkg, err := imp.Import("example")
	if err != nil {
		return fmt.Errorf("failed to import package: %v", err)
	}

	// Display package information
	fmt.Println("=== Package Information ===")
	fmt.Printf("Name: %s\n", pkg.Name())
	fmt.Printf("Path: %s\n", pkg.Path())
	fmt.Printf("Complete: %v\n", pkg.Complete())
	fmt.Println()

	// Display imports
	imports := pkg.Imports()
	if len(imports) > 0 {
		fmt.Println("=== Imports ===")
		for _, imp := range imports {
			fmt.Printf("  %s\n", imp.Path())
		}
		fmt.Println()
	}

	// Display exported objects
	fmt.Println("=== Exported Declarations ===")
	scope := pkg.Scope()
	names := scope.Names()

	if len(names) == 0 {
		fmt.Println("  (no exported objects)")
		fmt.Println()
		return nil
	}

	// Group objects by kind
	consts := []types.Object{}
	vars := []types.Object{}
	funcs := []types.Object{}
	typesObj := []types.Object{}

	for _, name := range names {
		obj := scope.Lookup(name)
		switch obj.(type) {
		case *types.Const:
			consts = append(consts, obj)
		case *types.Var:
			vars = append(vars, obj)
		case *types.Func:
			funcs = append(funcs, obj)
		case *types.TypeName:
			typesObj = append(typesObj, obj)
		}
	}

	// Print constants
	if len(consts) > 0 {
		fmt.Println("Constants:")
		for _, obj := range consts {
			c := obj.(*types.Const)
			fmt.Printf("  const %s %s = %s\n", c.Name(), c.Type(), c.Val())
		}
		fmt.Println()
	}

	// Print variables
	if len(vars) > 0 {
		fmt.Println("Variables:")
		for _, obj := range vars {
			v := obj.(*types.Var)
			fmt.Printf("  var %s %s\n", v.Name(), v.Type())
		}
		fmt.Println()
	}

	// Print types
	if len(typesObj) > 0 {
		fmt.Println("Types:")
		for _, obj := range typesObj {
			tn := obj.(*types.TypeName)
			fmt.Printf("  type %s %s\n", tn.Name(), formatType(tn.Type()))

			// For named types, show methods
			if named, ok := tn.Type().(*types.Named); ok {
				if named.NumMethods() > 0 {
					for i := 0; i < named.NumMethods(); i++ {
						method := named.Method(i)
						fmt.Printf("      func (%s) %s%s\n", tn.Name(), method.Name(), formatSignature(method.Type().(*types.Signature)))
					}
				}
			}
		}
		fmt.Println()
	}

	// Print functions
	if len(funcs) > 0 {
		fmt.Println("Functions:")
		for _, obj := range funcs {
			f := obj.(*types.Func)
			sig := f.Type().(*types.Signature)
			fmt.Printf("  func %s%s\n", f.Name(), formatSignature(sig))
		}
		fmt.Println()
	}

	return nil
}

// formatType formats a type for display
func formatType(t types.Type) string {
	switch typ := t.(type) {
	case *types.Named:
		// For named types, show the underlying type
		return typ.Obj().Name() + " " + formatType(typ.Underlying())
	case *types.Struct:
		if typ.NumFields() == 0 {
			return "struct{}"
		}
		result := "struct {\n"
		for i := 0; i < typ.NumFields(); i++ {
			field := typ.Field(i)
			result += fmt.Sprintf("      %s %s\n", field.Name(), field.Type())
		}
		result += "    }"
		return result
	case *types.Interface:
		if typ.NumMethods() == 0 {
			return "interface{}"
		}
		result := "interface {\n"
		for i := 0; i < typ.NumMethods(); i++ {
			method := typ.Method(i)
			sig := method.Type().(*types.Signature)
			result += fmt.Sprintf("      %s %s\n", method.Name(), formatSignature(sig))
		}
		result += "    }"
		return result
	default:
		return t.String()
	}
}

// formatSignature formats a function signature
func formatSignature(sig *types.Signature) string {
	params := formatTuple(sig.Params(), sig.Variadic(), true)
	results := formatTuple(sig.Results(), false, false)

	if results == "" || results == "()" {
		return params
	}
	return params + " " + results
}

// formatTuple formats a parameter or result tuple
// isParams indicates whether this is a parameter tuple (always needs parens) or result tuple
func formatTuple(tuple *types.Tuple, variadic bool, isParams bool) string {
	if tuple.Len() == 0 {
		if isParams {
			return "()"
		}
		return ""
	}

	parts := make([]string, tuple.Len())
	for i := 0; i < tuple.Len(); i++ {
		v := tuple.At(i)
		typeStr := v.Type().String()

		// Handle variadic parameter
		if variadic && i == tuple.Len()-1 {
			// Convert []T to ...T
			if strings.HasPrefix(typeStr, "[]") {
				typeStr = "..." + typeStr[2:]
			}
		}

		if v.Name() != "" {
			parts[i] = v.Name() + " " + typeStr
		} else {
			parts[i] = typeStr
		}
	}

	result := "(" + strings.Join(parts, ", ") + ")"

	// If single unnamed result (not params), omit the parentheses
	if !isParams && tuple.Len() == 1 && tuple.At(0).Name() == "" {
		return tuple.At(0).Type().String()
	}

	return result
}

// typeCodeName returns a human-readable name for a type code
func typeCodeName(code pkgbits.CodeType) string {
	switch code {
	case pkgbits.TypeBasic:
		return "Basic"
	case pkgbits.TypeNamed:
		return "Named"
	case pkgbits.TypePointer:
		return "Pointer"
	case pkgbits.TypeSlice:
		return "Slice"
	case pkgbits.TypeArray:
		return "Array"
	case pkgbits.TypeChan:
		return "Chan"
	case pkgbits.TypeMap:
		return "Map"
	case pkgbits.TypeSignature:
		return "Signature"
	case pkgbits.TypeStruct:
		return "Struct"
	case pkgbits.TypeInterface:
		return "Interface"
	case pkgbits.TypeUnion:
		return "Union"
	case pkgbits.TypeTypeParam:
		return "TypeParam"
	default:
		return fmt.Sprintf("Unknown(%d)", code)
	}
}

// objTagName returns the name of an object tag
func objTagName(tag pkgbits.CodeObj) string {
	switch tag {
	case pkgbits.ObjAlias:
		return "Alias"
	case pkgbits.ObjConst:
		return "Const"
	case pkgbits.ObjType:
		return "Type"
	case pkgbits.ObjFunc:
		return "Func"
	case pkgbits.ObjVar:
		return "Var"
	case pkgbits.ObjStub:
		return "Stub"
	default:
		return fmt.Sprintf("Unknown(%d)", tag)
	}
}
