# Go Unified IR Export Format

This document explains the Unified IR (UIR) format used in Go's `__.PKGDEF` files, based on investigation of the Go compiler source code.

## Overview

The Unified IR format is Go's binary export data format introduced in Go 1.17+ as part of the migration to a unified intermediate representation. It stores package export information that other packages need during compilation for type checking and symbol resolution.

## File Structure

### Archive Level

A compiled Go package (`.a` file) is a Unix AR archive with the following structure:

```
!<arch>\n                    # Archive magic (8 bytes)
[60-byte header]             # Archive entry header
__.PKGDEF                    # Export data section
[padding if odd size]
[60-byte header]             # Archive entry header
_go_.o                       # Object code section
[padding if odd size]
```

### __.PKGDEF Structure

The `__.PKGDEF` section contains:

```
go object <os> <arch> <version> <experiments>\n
\n$$B\n                      # Binary export marker
u                            # Unified IR format marker
<binary unified IR data>     # Actual export data
\n$$\n                       # End marker
```

Example header:
```
go object darwin amd64 go1.23 X:regabireflect,regabiwrappers\n
```

## Binary Format

### Overall Structure

The Unified IR binary format consists of:

```
[4 bytes] Version            # Format version number (little-endian uint32)
[4 bytes] Flags              # Optional flags (if version supports)
[40 bytes] Section Ends      # End offsets for 10 section types (uint32 array)
[variable] Element Ends      # End offsets for all elements
[variable] Element Data      # Actual data payloads
[8 bytes] Fingerprint        # Package fingerprint
```

### Sections

The format divides data into 10 section types:

| Section | Index | Purpose |
|---------|-------|---------|
| String | 0 | Deduplicated strings |
| Meta | 1 | Metadata (public & private roots) |
| PosBase | 2 | Position information (file names) |
| Pkg | 3 | Package paths |
| Name | 4 | (Reserved) |
| Type | 5 | Type definitions |
| Obj | 6 | Object declarations |
| ObjExt | 7 | Extended object info |
| ObjDict | 8 | Object dictionaries |
| Body | 9 | Function bodies (private) |

### Primitive Encoding

Primitives use variable-width encoding:

```
Bool    = byte              # 0 or 1
Int64   = zvarint           # Zig-zag encoded signed varint
Uint64  = uvarint           # Unsigned varint
String  = [bytes]           # Raw bytes, length determined by element bounds
```

Variable-width integers (varints):
- Use 7 bits per byte for data
- 8th bit indicates if more bytes follow
- Compact for small numbers (1 byte for 0-127)
- Zig-zag encoding for signed numbers maps negatives efficiently

### Reference Tables

Each element is preceded by a reference table that lists all other elements it references:

```
[uvarint] Table Length      # Number of references
{
  [uvarint] Section Kind    # Which section (0-9)
  [uvarint] Element Index   # Relative index within section
} * Table Length
[element data]              # Actual element content
```

Benefits:
- First reference: 3-4 bytes (section + index + length)
- Subsequent references: 1 byte (table index)
- Enables efficient UIR linking without parsing element data
- Allows updating references during linking

### Sync Markers

Optional sync markers help debug encoder/decoder mismatches:

```
Sync = [uvarint] Marker ID  # Expected sync point type
       [uvarint] PC Count   # Number of program counters
       { [uvarint] PC } *   # Encoder PCs for debugging
```

Common markers:
- `SyncPublic` - Start of public root
- `SyncPrivate` - Start of private root
- `SyncPkg` - Package reference
- `SyncObject` - Object declaration
- `SyncType` - Type reference
- `SyncEOF` - End of section

## Encoding Details

### Constants

Constants are encoded with their type and value:

```
Constant = [Sync]
           Bool            # Is complex?
           Scalar          # Real part (or only value)
           [Scalar]        # Imaginary part if complex
```

Scalar encoding depends on value kind:

```
Scalar = [Sync]
         Uint64            # Value kind code
         Value             # Depends on kind:
                          #   Bool: byte
                          #   Int64: zvarint
                          #   String: StringRef
                          #   BigInt: Term (bytes + sign)
                          #   BigRat: Term Term (numerator/denominator)
                          #   BigFloat: BigBytes (precision 512)
```

### Types

Type encoding depends on the type kind:

```
Type = [Sync]
       CodeType           # Type kind code

CodeType:
  TypeBasic      = 0      # Builtin type (int, string, etc.)
  TypeNamed      = 1      # Named type with methods
  TypePointer    = 2      # Pointer type
  TypeSlice      = 3      # Slice type
  TypeArray      = 4      # Array type
  TypeChan       = 5      # Channel type
  TypeMap        = 6      # Map type
  TypeSignature  = 7      # Function signature
  TypeStruct     = 8      # Struct type
  TypeInterface  = 9      # Interface type
  TypeUnion      = 10     # Type union (generics)
  TypeTypeParam  = 11     # Type parameter (generics)
```

Example - Named Type:
```
[Sync]
TypeNamed
[Sync] Sym
  [Sync] Pkg
    [Reloc] Package Index
  [Sync] Name
    [Reloc] String Index (type name)
[Reloc] Underlying Type Index
[Uint64] Number of Type Parameters
{ [Reloc] Type Parameter Index } *
[Uint64] Number of Methods
{ [Reloc] Method Index } *
```

Example - Function Signature:
```
[Sync]
TypeSignature
[Bool] Has Receiver?
  [Sync] Type
    [Reloc] Receiver Type Index
[Uint64] Number of Parameters
{
  [Sync] Type
    [Reloc] Parameter Type Index
} *
[Bool] Is Variadic?
[Uint64] Number of Results
{
  [Sync] Type
    [Reloc] Result Type Index
} *
```

### Objects

Objects (package-level declarations) have a tag indicating their kind:

```
Object = [Sync]
         CodeObj           # Object kind

CodeObj:
  ObjAlias  = 0           # Type alias
  ObjConst  = 1           # Constant
  ObjType   = 2           # Type definition
  ObjFunc   = 3           # Function
  ObjVar    = 4           # Variable
  ObjStub   = 5           # Import stub
```

Example - Function Object:
```
[Sync] Object1
[Sync] Sym
  [Sync] Pkg
    [Reloc] Package Index
  [Sync] Name
    [Reloc] String Index (function name)
[Sync] Type
  [Reloc] Signature Type Index
[Bool] Has Type Parameters?
  [Uint64] Number of Type Parameters
  { [Reloc] Type Parameter Index } *
```

Example - Const Object:
```
[Sync] Object1
[Sync] Sym
  [Sync] Pkg
    [Reloc] Package Index
  [Sync] Name
    [Reloc] String Index (const name)
[Sync] Type
  [Reloc] Type Index
[Sync] Value
  [Constant] Value Encoding
```

### Root Sections

The format has two root metadata entries:

#### Public Root (Index 0 in Meta section)

Contains exported declarations:

```
[Sync] Public
[Sync] Pkg
  [Reloc] Self Package Index
[Bool] Has Init? (if version supports)
[Uint64] Number of Exported Objects
{
  [Sync] Object
  [Bool] Is Derived? (if version supports)
  [Reloc] Object Index
  [Uint64] Extra Count (usually 0)
} *
[Sync] EOF
```

#### Private Root (Index 1 in Meta section)

Contains function bodies and internal data:

```
[Bool] Has .inittask?
[Uint64] Number of Bodies
{
  [String] Package Path
  [String] Symbol Name
  [Reloc] Body Index
} *
[Sync] EOF
```

## Compilation Process

### Writing Phase (Compiler)

1. **Setup** (`src/cmd/compile/internal/noder/unified.go:466-478`):
   ```go
   version := pkgbits.V1  // or V2 with experiments
   l := linker{
       pw: pkgbits.NewPkgEncoder(version, syncFrames),
       pkgs: make(map[string]index),
       decls: make(map[*types.Sym]index),
   }
   ```

2. **Create Roots** (`unified.go:480-483`):
   - Public root encoder (index 0)
   - Private root encoder (index 1)

3. **Export Objects** (`unified.go:488-516`):
   - Read local package metadata
   - Filter exported symbols
   - Relocate to output via linker

4. **Write Public Root** (`unified.go:518-546`):
   ```go
   w.Sync(SyncPkg)
   w.Reloc(SectionPkg, selfPkgIdx)
   w.Len(len(exportedObjects))
   for each exported object:
       w.Sync(SyncObject)
       w.Reloc(SectionObj, objIdx)
   ```

5. **Write Private Root** (`unified.go:548-572`):
   - Check for init function
   - Write function bodies
   - Used for inlining in same package

6. **Finalize** (`unified.go:574`):
   ```go
   fingerprint := encoder.DumpTo(out)
   ```

### Archive Creation

The compiler creates the archive (`src/cmd/compile/internal/gc/obj.go:53-79`):

```go
func dumpobj1(outfile string, mode int) {
    fmt.Fprintf(bout, "!<arch>\n")

    // Write __.PKGDEF
    startArchiveEntry(bout)
    dumpCompilerObj(bout)  // Calls WriteExports
    finishArchiveEntry(bout, start, "__.PKGDEF")

    // Write _go_.o
    startArchiveEntry(bout)
    dumpLinkerObj(bout)    // Calls obj.WriteObjFile
    finishArchiveEntry(bout, start, "_go_.o")
}
```

### Cache Storage

The build system caches archives (`src/cmd/go/internal/work/buildid.go:722`):

```go
// Compute action ID from inputs
actionID := hash(sourceFiles, flags, dependencies)

// Store in cache
outputID, _, err := cache.Put(actionID, archiveReader)

// Path: $GOCACHE/{actionID} -> archive file
// Archive contains: buildID = actionID/contentID
```

### Reading Phase (Compiler importing a package)

1. **Check Cache** (`src/cmd/go/internal/work/buildid.go:545`):
   ```go
   if file, _, err := cache.GetFile(c, actionHash); err == nil {
       // Use cached archive
       a.built = file
   }
   ```

2. **Extract Export Data**:
   - Open archive file
   - Locate `__.PKGDEF` entry
   - Extract data between `\n$$B\n` and `\n$$\n`
   - Verify `u` prefix for Unified IR

3. **Decode** (`src/go/internal/gcimporter/ureader.go:48`):
   ```go
   input := pkgbits.NewPkgDecoder(pkgPath, exportData)
   pr := pkgReader{PkgDecoder: input}
   r := pr.newReader(SectionMeta, PublicRootIdx, SyncPublic)
   pkg := r.pkg()

   // Read exported objects
   for i := 0; i < r.Len(); i++ {
       r.Sync(SyncObject)
       objIdx := r.Reloc(SectionObj)
       pr.objIdx(objIdx)  // Creates types.Object
   }
   ```

4. **Type Checking**:
   - Use imported `go/types.Package`
   - Validate interface implementations
   - Resolve method calls
   - Check generic constraints

### Linking Phase

The linker processes archives (`src/cmd/link/internal/ld/lib.go:1100`):

```go
// For each input archive:
for each archive entry {
    if arhdr.name == "__.PKGDEF" {
        continue  // Skip export data, only needed by compiler
    }
    if arhdr.name == "_go_.o" {
        ldobj(ctxt, f, lib, l, pname, file)  // Load object
    }
}
```

Object loading:
- Reads symbol definitions
- Processes relocations
- Builds symbol table
- Resolves cross-references
- Generates final binary

## Version Evolution

### V1 (Go 1.17-1.22)

Base unified IR format with:
- 10 section types
- Basic sync markers
- Type parameters for generics

### V2 (Go 1.23+)

Added support for:
- Alias type parameters (experiment: `aliastypeparams`)
- Improved generic type handling
- Enhanced debugging information

Version selection (`unified.go:468-471`):
```go
version := pkgbits.V1
if buildcfg.Experiment.AliasTypeParams {
    version = pkgbits.V2
}
```

## Benefits of Unified IR

1. **Compact**: Variable-width encoding, string deduplication
2. **Fast**: Direct binary format, no parsing overhead
3. **Incremental**: Reference tables enable efficient linking
4. **Stable**: Section-based design allows format evolution
5. **Debuggable**: Optional sync markers catch encoder/decoder bugs
6. **Cached**: Content-addressed storage enables build caching

## Comparison with Previous Format

### Old Export Data (Pre-1.17)

- Text-based S-expression format
- Larger file sizes
- Slower to parse
- Less structured

### Unified IR (1.17+)

- Binary format
- ~30-50% smaller
- ~2-3x faster to parse
- Structured sections
- Better suited for generics

## Tools and Debugging

### Viewing Export Data

```bash
# Extract __.PKGDEF from archive
ar x package.a __.PKGDEF

# View with custom decoder (like the pkgdef-decoder tool)
./pkgdef-decoder package.a

# Use Go's internal tools
go tool compile -S file.go    # See assembly + export data size
```

### Cache Inspection

```bash
# Find cache location
go env GOCACHE

# List cached builds
ls -la $(go env GOCACHE)

# View specific cached package
go tool buildid <cache-file>
```

### Debug Flags

```bash
# Show export size
go build -gcflags='-m -m'

# Verbose build
go build -x -v
```

## References

Key source files in the Go repository:

- `src/internal/pkgbits/doc.go` - Format documentation
- `src/internal/pkgbits/codes.go` - Type and object codes
- `src/internal/pkgbits/encoder.go` - Binary encoder
- `src/internal/pkgbits/decoder.go` - Binary decoder
- `src/cmd/compile/internal/noder/unified.go` - Export writing
- `src/cmd/compile/internal/noder/reader.go` - Import reading
- `src/go/internal/gcimporter/ureader.go` - go/types integration
