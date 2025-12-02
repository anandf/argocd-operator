# Template System Dependencies

The template-based architecture requires the following Go dependencies:

## Required Dependencies

### Sprig v3
**Package:** `github.com/Masterminds/sprig/v3`
**Purpose:** Provides template functions for Go templates
**License:** MIT
**Documentation:** http://masterminds.github.io/sprig/

Add to `go.mod`:
```bash
go get github.com/Masterminds/sprig/v3
```

This library provides 100+ template functions including:
- String manipulation (upper, lower, trim, quote, etc.)
- Type conversion (toString, toInt, etc.)
- Encoding (b64enc, b64dec, etc.)
- Default values (default, coalesce, etc.)
- Date/time functions
- Cryptographic functions
- And many more...

### Already Available (No Action Needed)

The following dependencies are already part of the operator:

- **k8s.io/apimachinery** - For Kubernetes types and YAML parsing
- **sigs.k8s.io/controller-runtime** - For client.Object interface
- **text/template** - Go standard library for template rendering
- **embed** - Go standard library for embedding files

## Installation

To install all dependencies:

```bash
cd /Users/anjoseph/go/src/github.com/anandf/argocd-operator
go get github.com/Masterminds/sprig/v3
go mod tidy
```

## Verification

Verify the dependency is installed:

```bash
go list -m github.com/Masterminds/sprig/v3
```

Expected output:
```
github.com/Masterminds/sprig/v3 v3.2.3
```

## Usage in Code

Import Sprig in your template engine:

```go
import (
    "text/template"
    "github.com/Masterminds/sprig/v3"
)

// Create template with Sprig functions
tmpl := template.New("example").
    Funcs(sprig.TxtFuncMap()).
    Parse(templateString)
```

## License Compatibility

Sprig is licensed under MIT, which is compatible with Apache 2.0 (the ArgoCD Operator license).

**MIT License:**
- ✅ Commercial use allowed
- ✅ Modification allowed
- ✅ Distribution allowed
- ✅ Private use allowed
- ⚠️ Must include license and copyright notice

## Alternative Approaches

If you prefer not to add the Sprig dependency, you can:

1. **Use only built-in Go template functions**
   - Limited to: `and`, `or`, `not`, `eq`, `ne`, `lt`, `le`, `gt`, `ge`, `index`, `len`, `print`, `printf`, `println`
   - No string manipulation or encoding functions

2. **Write custom template functions**
   ```go
   funcMap := template.FuncMap{
       "upper": strings.ToUpper,
       "lower": strings.ToLower,
       "quote": strconv.Quote,
       // ... add more as needed
   }
   tmpl := template.New("example").Funcs(funcMap)
   ```

3. **Pre-process data in Go**
   - Do all transformations in the data builder
   - Keep templates purely for structure

However, Sprig is widely used in the Kubernetes ecosystem (Helm uses it) and is well-maintained.

## Security Considerations

Sprig includes some functions that could be security-sensitive:

- **File system access:** Sprig doesn't include file system functions by default in `TxtFuncMap()`
- **Command execution:** Not included in Sprig
- **Network access:** Not included in Sprig

The template data is controlled by the operator code, not by user input, so template injection attacks are not a concern.

## Size Impact

Adding Sprig to your binary will increase its size by approximately:
- **Compressed:** ~150 KB
- **Uncompressed:** ~500 KB

This is negligible for a Kubernetes operator binary.

## Version Pinning

It's recommended to pin to a specific version in `go.mod`:

```go
require (
    github.com/Masterminds/sprig/v3 v3.2.3
)
```

## Updates

Check for updates periodically:

```bash
go list -u -m github.com/Masterminds/sprig/v3
```

Update when needed:

```bash
go get -u github.com/Masterminds/sprig/v3
go mod tidy
```
