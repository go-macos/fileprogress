// Copyright (c) 2026, the go-macos authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file.

package fileprogress

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/go-macos/objc"
)

// ErrUnavailable means the system would not hand over what publishing needs.
// It is worth reporting and never worth failing a download over: a file that
// arrives without a pie has still arrived.
var ErrUnavailable = errors.New("fileprogress: the system will not publish progress")

// keys are the four Foundation constants a published file progress is
// described with. They are read from the framework rather than written down:
// a string that is wrong by one character is not an error, it is a
// publication nobody subscribes to, which looks exactly like this package
// doing nothing.
type keys struct {
	kindFile objc.ID
	opKey    objc.ID
	opDown   objc.ID
	urlKey   objc.ID
}

var (
	once   sync.Once
	loaded keys
	err    error

	// Seams, in the shape this organisation already uses for its bridges.
	// Every one of these can fail on somebody's machine and none of them
	// can be made to fail on purpose here, so they are staged instead: an
	// error path nobody has run is an error path nobody has checked.
	loadFn      = load
	dlopenFn    = func(path string) (uintptr, error) { return purego.Dlopen(path, purego.RTLD_LAZY|purego.RTLD_GLOBAL) }
	dlsymFn     = purego.Dlsym
	frameworkFn = objc.Load
	registerFn  = func(h uintptr) { purego.RegisterLibFunc(&memmove, h, "memmove") }
	// newProgress is the last of them: a system that refuses to make the
	// object at all cannot be arranged, and a caller must still be told
	// rather than handed something that quietly publishes nothing.
	newProgress = func(total int64) objc.ID {
		return objc.ClassID("NSProgress").Send(objc.Sel("progressWithTotalUnitCount:"), total)
	}
)

// memmove copies from an address the dynamic linker gave us into memory Go
// owns. The value is fetched this way rather than by turning the address into
// a pointer, so that go vet never has to be told to look away: what dlsym
// hands back is a number, and the one place it is treated as memory is here.
var memmove func(dst unsafe.Pointer, src uintptr, n uintptr)

// constant reads an exported NSString * const. dlsym gives the address of the
// pointer, not the string, so what lives there is copied out.
func constant(h uintptr, name string) (objc.ID, error) {
	addr, err := dlsymFn(h, name)
	if err != nil || addr == 0 {
		return 0, fmt.Errorf("%w: %s: %v", ErrUnavailable, name, err)
	}
	var id objc.ID
	memmove(unsafe.Pointer(&id), addr, unsafe.Sizeof(id))
	if id == 0 {
		return 0, fmt.Errorf("%w: %s is empty", ErrUnavailable, name)
	}
	return id, nil
}

func load() (keys, error) {
	if err := frameworkFn(objc.Foundation); err != nil {
		return keys{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	h, derr := dlopenFn(objc.Foundation)
	if derr != nil {
		return keys{}, fmt.Errorf("%w: %v", ErrUnavailable, derr)
	}
	sys, derr := dlopenFn(objc.LibSystem)
	if derr != nil {
		return keys{}, fmt.Errorf("%w: %v", ErrUnavailable, derr)
	}
	registerFn(sys)
	var k keys
	for _, spec := range []struct {
		name string
		into *objc.ID
	}{
		{"NSProgressKindFile", &k.kindFile},
		{"NSProgressFileOperationKindKey", &k.opKey},
		{"NSProgressFileOperationKindDownloading", &k.opDown},
		{"NSProgressFileURLKey", &k.urlKey},
	} {
		v, err := constant(h, spec.name)
		if err != nil {
			return keys{}, err
		}
		*spec.into = v
	}
	return k, nil
}

// Progress is a download the system has been told about.
//
// The zero value is usable and does nothing, so a caller on a machine that
// would not publish need not treat it as a special case.
type Progress struct {
	mu  sync.Mutex
	obj objc.ID
}

// Publish tells the system that the file at path is being downloaded, and how
// many bytes are expected. Call Done when it is over, however it ends.
func Publish(path string, total int64) (*Progress, error) {
	once.Do(func() { loaded, err = loadFn() })
	if err != nil {
		return &Progress{}, err
	}
	if path == "" {
		return &Progress{}, fmt.Errorf("%w: a download with no file", ErrUnavailable)
	}
	obj := newProgress(total)
	if obj == 0 {
		return &Progress{}, fmt.Errorf("%w: NSProgress refused", ErrUnavailable)
	}
	// Held on to deliberately. What a class method hands back is
	// autoreleased, and a published progress that is freed at the end of the
	// current pool stops being published — silently, and only sometimes,
	// which is the worst way for it to fail.
	obj = obj.Send(objc.Sel("retain"))
	obj.Send(objc.Sel("setKind:"), loaded.kindFile)
	url := objc.ClassID("NSURL").Send(objc.Sel("fileURLWithPath:"), objc.NSString(path))
	obj.Send(objc.Sel("setUserInfoObject:forKey:"), url, loaded.urlKey)
	obj.Send(objc.Sel("setUserInfoObject:forKey:"), loaded.opDown, loaded.opKey)
	// Nothing here can honour a cancel or a pause asked for from the Finder,
	// so it does not offer them: a button that does nothing is worse than no
	// button.
	obj.Send(objc.Sel("setCancellable:"), false)
	obj.Send(objc.Sel("setPausable:"), false)
	obj.Send(objc.Sel("publish"))
	return &Progress{obj: obj}, nil
}

// Set says how many bytes have arrived.
func (p *Progress) Set(done int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.obj == 0 {
		return
	}
	p.obj.Send(objc.Sel("setCompletedUnitCount:"), done)
}

// Done takes the download off the system's books, whether it finished or not.
// It is safe to call more than once, so a caller can defer it and also call it
// where the work really ends.
func (p *Progress) Done() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.obj == 0 {
		return
	}
	p.obj.Send(objc.Sel("unpublish"))
	p.obj.Send(objc.Sel("release"))
	p.obj = 0
}
