// Copyright (c) 2026, the go-macos authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file.

//go:build !darwin

package fileprogress

import "errors"

// ErrUnavailable means the system will not publish a file's progress. Away
// from macOS that is every system, which is not a fault worth reporting twice.
var ErrUnavailable = errors.New("fileprogress: only macOS publishes a file's progress")

// Progress is a download nobody was told about.
type Progress struct{}

// Publish does nothing away from macOS, and says so once.
func Publish(string, int64) (*Progress, error) { return &Progress{}, ErrUnavailable }

// Set does nothing.
func (p *Progress) Set(int64) {}

// Done does nothing.
func (p *Progress) Done() {}
