// unified-ir-reader reads and decodes __.PKGDEF files from Go .a archives
// and prints their contents in human-readable format.
package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
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

	// Show binary format information
	if err := showDetailedFormat(uirData, *limit); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding format: %v\n", err)
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
	fmt.Println("║                   Unified IR Binary Format                    ║")
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
