# 🔍 unified-ir-reader

> Peek inside Go's compiler cache and see what's really happening behind the scenes

Ever wondered what the Go compiler actually stores in those `.a` archive files? This tool decodes the internal format and shows you exactly what's inside—in plain English.

**Built for [Internals for Interns](https://internals-for-interns.com)** • **Works with Go 1.25**

---

## 🚀 Quick Start

```bash
# Install
go install github.com/jespino/unified-ir-reader@latest

# Use it
unified-ir-reader path/to/package.a

# For large packages, limit the output
unified-ir-reader --limit 10 path/to/package.a
```

---

## 💡 What Does It Do?

When you compile a Go program, the compiler creates `.a` archive files that contain two things:

- **`__.PKGDEF`** — The package's "contract": what functions, types, and constants it exports
- **`_go_.o`** — The actual machine code that runs

This tool reads the `__.PKGDEF` section and shows you:
- All the strings used in your package
- What other packages it depends on
- Type definitions and function signatures
- Which functions can be inlined
- And much more!

---

## 📊 Example Output

```
╔═══════════════════════════════════════════════════════════════╗
║                   Unified IR Binary Format                    ║
╚═══════════════════════════════════════════════════════════════╝

=== Format Metadata ===
Sync Markers: false
Total Elements: 256
Fingerprint: 8843588eb035041c

=== Section Statistics ===
  SectionString   :   67 elements
  SectionMeta     :    2 elements
  SectionPkg      :    2 elements
  SectionType     :   61 elements
  SectionObj      :   29 elements
  SectionBody     :    7 elements
  ...

=== SectionString (Deduplicated Strings) ===
  [  0] ""
  [  1] "complex"
  [  2] "<unlinkable>"
  [  3] "/Users/.../complex.go"
  ...
```

---

## 🎯 What You'll See

### 📝 **SectionString** — All The Strings
Every string in your package, stored once. If "main" appears 500 times in your code, it's stored once here and referenced everywhere else.

### 📦 **SectionPkg** — Package Dependencies
The full dependency graph—not just the packages you import, but everything they import too.

### 🏗️ **SectionType** — Type Information
All the types: structs, interfaces, function signatures, generics, and more.

### 🔧 **SectionObj** — Exported Declarations
What your package exports: constants, variables, functions, and types.

### ⚡ **SectionBody** — Function Bodies
The actual code for functions that can be inlined across packages.

### 🎛️ **SectionMeta** — Compiler Metadata
Internal details like initialization tasks and optimization hints.

---

## 🛠️ Building From Source

```bash
# Clone the repo
git clone https://github.com/jespino/unified-ir-reader.git
cd unified-ir-reader

# Build
go build -o unified-ir-reader

# Run
./unified-ir-reader path/to/package.a
```

---

## 🧪 Creating Test Archives

Want to see how your own code looks in the Unified IR format?

```bash
# Write some Go code
cat > example.go << 'EOF'
package example

const Answer = 42

type Person struct {
    Name string
    Age  int
}

func Greet(name string) string {
    return "Hello, " + name
}
EOF

# Compile it
go tool compile -pack -o example.a example.go

# Decode it
unified-ir-reader example.a
```

---

## 📚 Learn More

Want to understand what's going on under the hood?

Check out the [Internals for Interns](https://internals-for-interns.com) blog for deep dives into:
- How the Go compiler works
- What the Unified IR format is
- Why compilation caching matters
- And much more!

### 🔗 Related Go Source Files

If you want to explore the compiler source:
- `src/cmd/compile/internal/noder/unified.go` — Where UIR gets written
- `src/internal/pkgbits/` — The binary format encoder/decoder
- `src/go/internal/gcimporter/` — How packages get imported

---

## ⚙️ Options

| Flag | Description |
|------|-------------|
| `--limit N` | Show only the first N entries per section (default: show all) |
| `--help` | Show usage information |

---

## 📖 About the Unified IR Format

The Unified IR (Unified Intermediate Representation) is Go's binary format for package metadata, introduced in Go 1.17. It's how the compiler stores and shares information between packages.

Think of it as a super-efficient filing system that:
- **Deduplicates** everything (store "main" once, reference it everywhere)
- **Organizes** data into specialized sections
- **Enables** fast compilation and cross-package optimization
- **Supports** modern features like generics

---

## 📄 License

Educational tool for exploring Go internals. The Go source code is governed by the Go project's BSD-style license.

---

<p align="center">
Made with ❤️ for Go learners everywhere
</p>
