# unified-ir-reader

A Go tool to decode and display the contents of `__.PKGDEF` files from Go `.a` archive files in human-readable format. Supports reading and parsing Go's Unified IR (UIR) export data format.

## What is __.PKGDEF?

When Go compiles a package, it creates a `.a` archive file containing two key components:

1. **`__.PKGDEF`** - Export data in Unified IR format containing:
   - Package metadata and type information
   - Exported symbols and their signatures
   - Interface definitions
   - Constants and their values
   - Used by the compiler during type checking of dependent packages

2. **`_go_.o`** - Object file containing:
   - Symbol definitions and machine code
   - Relocations and data sections
   - Used by the linker to create the final binary

## Unified IR Format

The Unified IR (UIR) format is Go's binary export data format introduced in Go 1.17+. It stores package export information in a compact, efficient binary representation that includes:

- Package path and name
- Exported constants, variables, functions, and types
- Type definitions including structs, interfaces, and function signatures
- Generic type parameters and constraints
- Build-time metadata

The format uses:
- Variable-width integers for compact storage
- Section-based organization (String, Meta, Pkg, Type, Obj, etc.)
- Reference tables for efficient cross-referencing
- Optional sync markers for debugging

## Usage

```bash
# Build the tool
go build -o unified-ir-reader

# Decode and display a .a archive file (shows all content)
./unified-ir-reader path/to/package.a

# Limit the number of entries shown per section
./unified-ir-reader --limit 10 path/to/package.a

# Show help
./unified-ir-reader --help
```

### Options

- `--limit N`: Limit the number of entries shown per section (0 = show all, default: 0)
  - Applies to: String table, Position bases, Package table, Function bodies
  - Useful for large packages to get a quick overview without overwhelming output

## Output

The tool displays comprehensive information about the Unified IR format:

```
╔═══════════════════════════════════════════════════════════════╗
║                   Unified IR Binary Format                    ║
╚═══════════════════════════════════════════════════════════════╝

=== Format Metadata ===
Sync Markers: false
Total Elements: 256
Fingerprint: 8843588eb035041c

=== Section Statistics ===
  String      :   67 elements
  Meta        :    2 elements
  PosBase     :    1 elements
  Pkg         :    2 elements
  Name        :   29 elements
  Type        :   61 elements
  Obj         :   29 elements
  ObjExt      :   29 elements
  ObjDict     :   29 elements
  Body        :    7 elements

=== String Table (Deduplicated Strings) ===
Total strings: 67
(showing first 50)

  [  0] ""
  [  1] "complex"
  [  2] "<unlinkable>"
  [  3] "/Users/.../complex.go"
  [  4] "ID"
  ...

=== Position Bases (Source Files) ===
  [0] /Users/.../complex.go (file base)

=== Package Table ===
  [0] <unlinkable> (name: complex)
  [1] builtin (system package)

=== Type Table ===
Total types: 61

=== Object Table Summary ===
Total objects: 29
  Const     : 5
  Alias     : 2
  Func      : 4
  Type      : 15
  Stub      : 3

=== Private Root (Function Bodies & Internal Data) ===
Has .inittask: false
Function bodies: 5

  [0] <unlinkable>.(*MyProcessor).Process (body index: 1)
  [1] <unlinkable>.(*MyProcessor).Close (body index: 2)
  ...
```

## Sections Displayed

The tool reveals the complete internal structure of the Unified IR format:

### Format Metadata
- **Sync Markers**: Whether debug sync markers are enabled
- **Total Elements**: Total number of elements across all sections
- **Fingerprint**: 8-byte package fingerprint for cache validation

### Section Statistics
Shows element counts for each of the 10 sections:
- **String**: Deduplicated strings (identifiers, paths, literals)
- **Meta**: Metadata including public and private roots
- **PosBase**: Position bases (source file paths)
- **Pkg**: Package references
- **Name**: Reserved section for names
- **Type**: Type definitions
- **Obj**: Object declarations (constants, variables, functions, types)
- **ObjExt**: Extended object information
- **ObjDict**: Object dictionaries for generics
- **Body**: Function bodies for inlining

### String Table
All strings used in the package, deduplicated and indexed. Includes:
- Package and import paths
- Identifier names (types, functions, fields, parameters)
- String literals from constants
- Special markers like "esc:" for escape analysis

### Position Bases
Source file paths where declarations are defined. Shows whether each position is a file base or line base (for `//line` directives).

### Package Table
All packages referenced by this package, including:
- The package itself (usually shown as `<self>`)
- Imported packages
- Built-in packages (`builtin`, `unsafe`)

### Type Table
Count of type definitions. Types include:
- Basic types (int, string, bool, etc.)
- Named types (custom types)
- Compound types (pointers, slices, arrays, maps, channels)
- Function signatures
- Structs and interfaces
- Generic type parameters

### Object Table Summary
Breakdown of exported objects by kind:
- **Const**: Constants
- **Var**: Variables
- **Func**: Functions
- **Type**: Type definitions
- **Alias**: Type aliases
- **Stub**: Import stubs for referenced external symbols

### Private Root
Internal data not exported but stored for compiler use:
- **Has .inittask**: Whether package has an init function
- **Function bodies**: List of functions with stored bodies for inlining
  - Shows package path, symbol name, and body index
  - Includes methods (e.g., `(*Type).Method`)

## How It Works

1. **Archive Parsing**: Reads the `.a` file and extracts the `__.PKGDEF` section
2. **Export Data Extraction**: Locates the Unified IR data between `\n$$B\n` and `\n$$\n` markers
3. **Binary Decoding**: Uses `internal/pkgbits` to decode the binary format and display all 10 sections
4. **Display**: Formats and displays the complete internal structure of the Unified IR format

## Implementation Details

The decoder:
- Parses the Unix archive format (AR) to extract `__.PKGDEF`
- Uses `internal/pkgbits` package (vendored from Go toolchain) for binary decoding
- Displays all 10 sections of the Unified IR format
- Shows string deduplication, package references, type information, and function bodies
- Applies configurable limits per section to manage output size

## Building Test Archives

To create a test archive:

```bash
# Create a test package
cat > example.go << 'EOF'
package example

const ExportedConst = 42

type ExportedType struct {
    Field int
}

func ExportedFunc() {}
EOF

# Compile to archive
go tool compile -pack -o example.a example.go
```

## Related Source Files

In the Go source tree:
- `src/cmd/compile/internal/noder/unified.go` - Unified IR writing
- `src/cmd/compile/internal/gc/obj.go` - Archive creation
- `src/internal/pkgbits/` - Binary encoding/decoding primitives
- `src/go/internal/gcimporter/` - Import functionality
- `src/cmd/go/internal/work/buildid.go` - Build cache management

## License

This tool is for educational purposes. The Go source code it references is governed by the Go project's BSD-style license.
