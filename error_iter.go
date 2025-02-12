// Copyright 2018 The LevelDB-Go and Pebble Authors. All rights reserved. Use
// of this source code is governed by a BSD-style license that can be found in
// the LICENSE file.

package pebble

import (
	"github.com/cockroachdb/pebble/internal/base"
	"github.com/cockroachdb/pebble/internal/keyspan"
)

type errorIter struct {
	err error
}

// errorIter implements the base.InternalIterator interface.
var _ internalIteratorWithStats = (*errorIter)(nil)

func newErrorIter(err error) *errorIter {
	return &errorIter{err: err}
}

func (c *errorIter) SeekGE(key []byte, trySeekUsingNext bool) (*InternalKey, []byte) {
	return nil, nil
}

func (c *errorIter) SeekPrefixGE(
	prefix, key []byte, trySeekUsingNext bool,
) (*base.InternalKey, []byte) {
	return nil, nil
}

func (c *errorIter) SeekLT(key []byte) (*InternalKey, []byte) {
	return nil, nil
}

func (c *errorIter) First() (*InternalKey, []byte) {
	return nil, nil
}

func (c *errorIter) Last() (*InternalKey, []byte) {
	return nil, nil
}

func (c *errorIter) Next() (*InternalKey, []byte) {
	return nil, nil
}

func (c *errorIter) Prev() (*InternalKey, []byte) {
	return nil, nil
}

func (c *errorIter) Error() error {
	return c.err
}

func (c *errorIter) Close() error {
	return c.err
}

func (c *errorIter) String() string {
	return "error"
}

func (c *errorIter) SetBounds(lower, upper []byte) {}
func (c *errorIter) Stats() InternalIteratorStats  { return InternalIteratorStats{} }
func (c *errorIter) ResetStats()                   {}

type errorKeyspanIter struct {
	err error
}

// errorKeyspanIter implements the keyspan.FragmentIterator interface.
var _ keyspan.FragmentIterator = (*errorKeyspanIter)(nil)

func newErrorKeyspanIter(err error) *errorKeyspanIter {
	return &errorKeyspanIter{err: err}
}

func (*errorKeyspanIter) SeekGE(key []byte) keyspan.Span    { return keyspan.Span{} }
func (*errorKeyspanIter) SeekLT(key []byte) keyspan.Span    { return keyspan.Span{} }
func (*errorKeyspanIter) First() keyspan.Span               { return keyspan.Span{} }
func (*errorKeyspanIter) Last() keyspan.Span                { return keyspan.Span{} }
func (*errorKeyspanIter) Next() keyspan.Span                { return keyspan.Span{} }
func (*errorKeyspanIter) Prev() keyspan.Span                { return keyspan.Span{} }
func (i *errorKeyspanIter) Clone() keyspan.FragmentIterator { return &errorKeyspanIter{err: i.err} }
func (i *errorKeyspanIter) Error() error                    { return i.err }
func (i *errorKeyspanIter) Close() error                    { return i.err }
func (*errorKeyspanIter) String() string                    { return "error" }
func (*errorKeyspanIter) SetBounds(lower, upper []byte)     {}
