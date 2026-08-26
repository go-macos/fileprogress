// Copyright (c) 2026, the go-macos authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file.

package fileprogress

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/go-macos/objc"
)

// reset puts the package back to never-loaded, so a test can drive the loading
// again. The seams are restored with it, because a test that leaves one in
// place breaks the next one in a way that reads as a failure of the code.
func reset(t *testing.T) {
	t.Helper()
	sf, sd, sy, sr, sg, sn := loadFn, dlopenFn, dlsymFn, registerFn, frameworkFn, newProgress
	t.Cleanup(func() {
		loadFn, dlopenFn, dlsymFn, registerFn, frameworkFn, newProgress = sf, sd, sy, sr, sg, sn
		once, loaded, err = sync.Once{}, keys{}, nil
	})
	once, loaded, err = sync.Once{}, keys{}, nil
}

// TestOnDevice_ADownloadIsPublishedAndTakenBack covers the whole life of a
// published download against the real system: the constants are read from
// Foundation, the object is made, kept, updated and taken back.
//
// What it cannot check is whether anything is drawn on screen — that took a
// person looking, and it is the only reason this package exists rather than
// the extended attribute it replaces, which wrote perfectly and did nothing.
func TestOnDevice_ADownloadIsPublishedAndTakenBack(t *testing.T) {
	reset(t)
	f := filepath.Join(t.TempDir(), "witness.mp4")
	if err := os.WriteFile(f, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Publish(f, 100)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if p.obj == 0 {
		t.Fatal("nothing was published")
	}
	for i := int64(0); i <= 100; i += 25 {
		p.Set(i)
		time.Sleep(5 * time.Millisecond)
	}
	p.Done()
	if p.obj != 0 {
		t.Error("the published progress was not taken back")
	}
	// Twice is not an error: a caller defers it and also calls it where the
	// work really ends.
	p.Done()
	p.Set(50)
}

// TestNothingIsPublishedForNothing covers the download with no file: a pie on
// a file nobody named is not something the system can draw.
func TestNothingIsPublishedForNothing(t *testing.T) {
	reset(t)
	p, err := Publish("", 100)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	// Still usable, so a caller need not treat the failure as a special
	// case: the zero value does nothing, quietly.
	p.Set(1)
	p.Done()
	var nothing *Progress
	nothing.Set(1)
	nothing.Done()
}

// TestASystemThatWillNotMakeTheObjectIsReported covers the machine that loads
// everything and still hands back nothing to publish.
func TestASystemThatWillNotMakeTheObjectIsReported(t *testing.T) {
	reset(t)
	newProgress = func(int64) objc.ID { return 0 }
	if _, err := Publish("/tmp/x.mp4", 10); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// TestASystemThatWillNotLoadIsReportedOnce covers the machine where the
// framework, the library or a constant will not come: publishing fails, says
// why, and is not retried on every download.
func TestASystemThatWillNotLoadIsReportedOnce(t *testing.T) {
	reset(t)
	calls := 0
	loadFn = func() (keys, error) {
		calls++
		return keys{}, errors.New("no Foundation here")
	}
	for range 3 {
		if _, err := Publish("/tmp/x.mp4", 10); err == nil {
			t.Fatal("publishing succeeded on a system that cannot load")
		}
	}
	if calls != 1 {
		t.Fatalf("the system was asked %d times, want once", calls)
	}
}

// TestEveryWayLoadingCanFail covers the four things that have to come back
// before anything can be published. Each is somebody's broken machine, and
// none can be made to fail here except on purpose.
func TestEveryWayLoadingCanFail(t *testing.T) {
	cases := map[string]func(){
		"the framework will not load": func() {
			frameworkFn = func(...string) error { return errors.New("refused") }
		},
		"the framework will not open": func() {
			dlopenFn = func(path string) (uintptr, error) { return 0, errors.New("refused") }
		},
		"the C library will not open": func() {
			calls := 0
			dlopenFn = func(path string) (uintptr, error) {
				calls++
				if calls == 1 {
					return 1, nil // Foundation opens
				}
				return 0, errors.New("refused") // libSystem does not
			}
			registerFn = func(uintptr) {}
		},
		"a constant is missing": func() {
			dlopenFn = func(path string) (uintptr, error) { return 1, nil }
			registerFn = func(uintptr) {}
			dlsymFn = func(uintptr, string) (uintptr, error) { return 0, errors.New("no such symbol") }
		},
		"a constant is empty": func() {
			dlopenFn = func(path string) (uintptr, error) { return 1, nil }
			dlsymFn = func(uintptr, string) (uintptr, error) { return 1, nil }
			// The symbol is there and holds nothing. Published with it, the
			// progress would describe itself to nobody — which looks
			// exactly like this package doing nothing at all.
			registerFn = func(uintptr) {
				memmove = func(unsafe.Pointer, uintptr, uintptr) {}
			}
		},
	}
	for name, stage := range cases {
		t.Run(name, func(t *testing.T) {
			reset(t)
			frameworkFn = func(...string) error { return nil }
			stage()
			if _, err := Publish("/tmp/x.mp4", 10); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
		})
	}
}
