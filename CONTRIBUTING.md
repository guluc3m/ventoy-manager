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
3. Set `Filename` to a case-insensitive regexp matching the ISO's real
   filename. Its first capture group must be the release version, and that
   group is what gets compared to decide which release is newest.
4. Set `Index` to the official page that lists releases and contains plain
   `href` links (a directory listing or a static download page).
5. If releases live in their own subdirectories, also set `Release` to a
   regexp matching those directory names, including the trailing slash as it
   appears in the page's links. Set `Path` when the ISO sits deeper inside the
   release directory. Leave both empty when the ISOs are listed directly at
   `Index`.

An image listed directly at the index:

```go
{
    Family:   "Example Linux",
    Filename: regexp.MustCompile(`(?i)^examplelinux-([0-9]+(?:\.[0-9]+)*)-x86_64\.iso$`),
    Index:    "https://downloads.example.org/releases/",
},
```

An image inside per-release directories (`releases/26.04/iso/`, ...):

```go
{
    Family:   "Example Linux",
    Edition:  "Desktop",
    Filename: regexp.MustCompile(`(?i)^examplelinux-([0-9]+(?:\.[0-9]+)*)-desktop-amd64\.iso$`),
    Index:    "https://downloads.example.org/releases/",
    Release:  regexp.MustCompile(`^[0-9]+\.[0-9]+/$`),
    Path:     "iso/",
},
```

After adding an entry:

- Add a sample filename to `TestCatalogParsesEveryImage` in
  `internal/iso/iso_test.go`.
- If the catalog's first entry or other ordering assertions change, update
  `TestCatalog` in the same file.
- Add a resolution test for the new index shape in
  `internal/source/source_test.go`, using `httptest` to serve a fake listing.
- Add the distribution to the supported-images list in `README.md`.
