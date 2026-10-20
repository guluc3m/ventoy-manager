# Contributing

Issues and pull requests are welcome. Before submitting a change, format the
code with `gofmt` and run the tests:

```sh
gofmt -l .
go test ./...
```


## Adding a distribution to the catalog

There is one catalog, `catalog` in `internal/iso/catalog.go`. The same entry
recognizes installed ISO filenames and tells the resolver where to find new
releases, so a new distribution needs exactly one entry there.

1. Add a `Distribution` to the `catalog` slice. 
2. Pick an identity: `Family`, plus `Edition` and/or `Flavor` when the family
   ships several recognizable images. The combination must be unique; it is
   what `Lookup` and the Ventoy alias use.
3. Set `Arches` to the architectures upstream publishes, in the order they
   should appear in the catalog. Use the canonical names `amd64` and `arm64`;
   the first entry is the default for filenames without an architecture token.
   Leave it empty only for an amd64-only distribution.
4. Set `Filename` to a case-insensitive regexp matching the ISO's real
   filename. Give the release version a named group `version` (use `version2`,
   `version3`, ... in regexp alternatives; the first populated one wins), and
   give the architecture a named group `arch` (`arch2`, ... in alternatives).
   Both are compared to decide which release is newest and which build an
   installed image should follow.
5. Set `Index` to the official page that lists releases and contains plain
   links (a directory listing, a static download page, or a page carrying the
   image URL in a `data-url` attribute). Links are resolved relative to
   `Index`, so absolute per-mirror URLs and SourceForge `/download` links work
   too. When an architecture lives in a different directory, add its own URL
   under `Indexes` (Debian does this).
6. If releases live in their own subdirectories, also set `Release` to a
   regexp matching those directory names, including the trailing slash as it
   appears in the page's links. Set `Path` when the ISO sits deeper inside the
   release directory, and `Paths` when several layouts exist; `PathsByArch`
   overrides both for architectures with their own directory (Fedora does
   this). Leave them empty when the ISOs are listed directly at `Index`.

An image listed directly at the index:

```go
{
    Family:   "Example Linux",
    Filename: regexp.MustCompile(`(?i)^examplelinux-(?P<version>[0-9]+(?:\.[0-9]+)*)-(?P<arch>x86_64|aarch64)\.iso$`),
    Arches:   []string{"amd64", "arm64"},
    Index:    "https://downloads.example.org/releases/",
},
```

An image inside per-release directories (`releases/26.04/iso/`, ...), with an
architecture-specific subdirectory:

```go
{
    Family:   "Example Linux",
    Edition:  "Desktop",
    Arches:   []string{"amd64", "arm64"},
    Filename: regexp.MustCompile(`(?i)^examplelinux-(?P<version>[0-9]+(?:\.[0-9]+)*)-desktop-(?P<arch>amd64|arm64)\.iso$`),
    Index:    "https://downloads.example.org/releases/",
    Release:  regexp.MustCompile(`^[0-9]+\.[0-9]+/$`),
    Path:     "iso/x86_64/",
    PathsByArch: map[string][]string{"arm64": {"iso/aarch64/"}},
},
```

After adding an entry:

- Add a sample filename, and one per supported architecture, to
  `TestCatalogParsesEveryImage` in `internal/iso/iso_test.go`.
- If the catalog's first entry or other ordering assertions change, update
  `TestCatalog` in the same file.
- Add a resolution test for the new index shape in
  `internal/source/source_test.go`, using `httptest` to serve a fake listing.
- Add the distribution to the supported-images list in `README.md`.
