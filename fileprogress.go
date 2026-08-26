// Copyright (c) 2026, the go-macos authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file.

// Package fileprogress tells macOS that a file is being downloaded, so the
// Finder draws the pie it draws for a browser's downloads.
//
// The pie is not something the system works out by watching a file grow:
// nothing about writing a file announces anything. It has to be published, and
// the system has exactly one way of hearing it — an NSProgress, published to
// whoever subscribes. An extended attribute on the file looks like it ought to
// work, is documented in various corners of the internet, and does nothing:
// that was measured against a real Finder before this package was written.
package fileprogress
