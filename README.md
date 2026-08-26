# fileprogress

[![CI](https://github.com/go-macos/fileprogress/actions/workflows/ci.yml/badge.svg)](https://github.com/go-macos/fileprogress/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-macos/fileprogress.svg)](https://pkg.go.dev/github.com/go-macos/fileprogress)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-macos/fileprogress)](https://goreportcard.com/report/github.com/go-macos/fileprogress)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE)

Pure-Go (`CGO_ENABLED=0`) publishing of a file's download progress to macOS, so
the Finder draws on the file's icon the pie it draws for a browser's downloads.
Reached through [`ebitengine/purego`](https://github.com/ebitengine/purego) and
[`go-macos/objc`](https://github.com/go-macos/objc) — `dlopen` + `objc_msgSend`
— so it links with **no cgo**.

```go
p, err := fileprogress.Publish(path, total)
if err != nil {
    // Worth logging, never worth failing a download over: a file that
    // arrives without a pie has still arrived.
}
defer p.Done()

for n := range bytesArriving {
    p.Set(n)
}
```

## The mechanism, and the one that looks like it should work

macOS infers nothing from a file growing on disk. The pie has to be published,
and the system hears it exactly one way: an `NSProgress` whose `kind` is
`NSProgressKindFile`, carrying the file's URL and a file-operation kind of
*downloading*, handed to `-publish`.

There is a second mechanism widely described as equivalent — setting the
extended attribute `com.apple.progress.fractionCompleted` on the file. **It does
not work.** Both were tried on the same machine against a real Finder, one at a
time, with a person looking: the attribute is written perfectly and draws
nothing; the published progress draws the pie. That measurement is why this
package exists, and it is worth repeating rather than trusting, because the
failure is silent — the attribute approach has tests that pass and a feature
that does nothing.

## Notes

- The published object is retained. What a class method hands back is
  autoreleased, and a published progress freed with the current pool stops being
  published silently and intermittently.
- `Done` is safe to call twice, so a caller can defer it and also call it where
  the work really ends.
- The four Foundation constants are read from the framework with `dlsym`, not
  written down: a name wrong by one character is not an error, it is a
  publication nobody subscribes to.
- Away from macOS every call is a no-op returning `ErrUnavailable` once.
