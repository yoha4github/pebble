// Copyright 2021 The LevelDB-Go and Pebble Authors. All rights reserved. Use
// of this source code is governed by a BSD-style license that can be found in
// the LICENSE file.

package rangekey

import (
	"github.com/cockroachdb/pebble/internal/base"
	"github.com/cockroachdb/pebble/internal/keyspan"
)

// Iterator iterates over coalesced range keys. It's implemented by Iter and
// DefragmentingIter.
type Iterator interface {
	Valid() bool
	Error() error
	SeekGE(key []byte) *CoalescedSpan
	SeekLT(key []byte) *CoalescedSpan
	First() *CoalescedSpan
	Last() *CoalescedSpan
	Next() *CoalescedSpan
	Prev() *CoalescedSpan
	Current() *CoalescedSpan
	Clone() Iterator
	Close() error
}

// TODO(jackson): Consider modifying the interface to support returning 'empty'
// spans that contain no range keys. This can avoid the need to open a range key
// block for a sstable in a distant part of the keyspace if there are no range
// keys in the vicinity of a read.
//
// Imagine you have a dense point keyspace a-z and a single range key covering
// the span [y, z). If you have an iterator without any bounds and perform a
// SeekGE('b'), the point iterator might land on an sstable with bounds like
// b-ba, but the rangekey iterator will need to scan all the sstables'
// fileMetadata until it finds the file containing [y, z) and opens it.
//
// Alternatively, the rangekey iterator could return a span [b, y) with no range
// keys, reflecting that it seeked and found that no range keys existed all the
// way up to y, all without reading any blocks. If the point iterator is
// eventually Next'd to y, the interleaving iterator can handle Next-ing the
// range key iterator onto the [y,z) range key's sstable and range key.

// This Iter implementation iterates over 'coalesced spans' that are not easily
// representable within the InternalIterator interface. Instead of iterating
// over internal keys, this Iter exposes CoalescedSpans that represent a set of
// overlapping fragments coalesced into a single internally consistent span.

// Iter is an iterator over a set of fragmented, coalesced spans. It wraps a
// keyspan.FragmentIterator containing fragmented keyspan.Spans with key kinds
// RANGEKEYSET, RANGEKEYUNSET and RANGEKEYDEL. The spans within the
// FragmentIterator must be sorted by Start key, including by decreasing
// sequence number if user keys are equal and key kind if sequence numbers are
// equal.
//
// Iter handles 'coalescing' spans on-the-fly, including dropping key spans that
// are no longer relevant.
type Iter struct {
	cmp           base.Compare
	formatKey     base.FormatKey
	visibleSeqNum uint64
	miter         keyspan.MergingIter
	iterSpan      keyspan.Span
	curr          CoalescedSpan
	err           error
	valid         bool
	dir           int8
}

// Assert that Iter implements the rangekey.Iterator interface.
var _ Iterator = (*Iter)(nil)

// Init initializes an iterator over a set of fragmented, coalesced spans.
func (i *Iter) Init(
	cmp base.Compare,
	formatKey base.FormatKey,
	visibleSeqNum uint64,
	iters ...keyspan.FragmentIterator,
) {
	*i = Iter{
		cmp:           cmp,
		formatKey:     formatKey,
		visibleSeqNum: visibleSeqNum,
	}
	i.miter.Init(cmp, keyspan.VisibleTransform(visibleSeqNum), iters...)
}

// Clone clones the iterator, returning an independent iterator over the same
// state. This method is temporary and may be deleted once range keys' state is
// properly reflected in readState.
func (i *Iter) Clone() Iterator {
	// TODO(jackson): Remove this method when the range keys' state is included
	// in the readState.
	// Init the new Iter to ensure err is cleared.
	newIter := &Iter{}
	newIter.Init(i.cmp, i.formatKey, i.visibleSeqNum,
		i.miter.ClonedIters()...)
	return newIter
}

// Error returns any accumulated error.
func (i *Iter) Error() error {
	return i.err
}

// Close closes all underlying iterators.
func (i *Iter) Close() error {
	return i.miter.Close()
}

func (i *Iter) coalesceForward() *CoalescedSpan {
	i.dir = +1
	if !i.iterSpan.Valid() {
		i.valid = false
		return nil
	}
	i.curr, i.err = Coalesce(i.cmp, i.iterSpan)
	if i.err != nil {
		i.valid = false
		return nil
	}
	i.valid = true
	return &i.curr
}

func (i *Iter) coalesceBackward() *CoalescedSpan {
	i.dir = -1
	if !i.iterSpan.Valid() {
		i.valid = false
		return nil
	}
	i.curr, i.err = Coalesce(i.cmp, i.iterSpan)
	if i.err != nil {
		i.valid = false
		return nil
	}
	i.valid = true
	return &i.curr
}

// SeekGE seeks the iterator to the first span covering a key greater than or
// equal to key and returns it.
func (i *Iter) SeekGE(key []byte) *CoalescedSpan {
	i.iterSpan = i.miter.SeekLT(key)
	if i.iterSpan.Valid() && i.cmp(key, i.iterSpan.End) < 0 {
		// We landed on a range key that begins before `key`, but extends beyond
		// it. Since we performed a SeekLT, we're on the last fragment with
		// those range key bounds and we need to coalesce backwards.
		return i.coalesceBackward()
	}
	// It's still possible that the next key is a range key with a start key
	// exactly equal to key. Move forward one. There's no point in checking
	// whether the next fragment actually covers the search key, because if it
	// doesn't it's still the first fragment covering a key ≥ the search key.
	i.iterSpan = i.miter.Next()
	return i.coalesceForward()
}

// SeekLT seeks the iterator to the first span covering a key less than key and
// returns it.
func (i *Iter) SeekLT(key []byte) *CoalescedSpan {
	i.iterSpan = i.miter.SeekLT(key)
	// We landed on the range key with the greatest start key that still sorts
	// before `key`.  Since we performed a SeekLT, we're on the last fragment
	// with those range key bounds and we need to coalesce backwards.
	return i.coalesceBackward()
}

// First seeks the iterator to the first span and returns it.
func (i *Iter) First() *CoalescedSpan {
	i.dir = +1
	i.iterSpan = i.miter.First()
	return i.coalesceForward()
}

// Last seeks the iterator to the last span and returns it.
func (i *Iter) Last() *CoalescedSpan {
	i.dir = -1
	i.iterSpan = i.miter.Last()
	return i.coalesceBackward()
}

// Next advances to the next span and returns it.
func (i *Iter) Next() *CoalescedSpan {
	if i.dir == +1 && !i.iterSpan.Valid() {
		// If we were already going forward and the underlying iterator is
		// invalid, there is no next item. Don't move the iterator, just
		// invalidate the iterator's position.
		i.valid = false
		return nil
	}
	i.dir = +1
	i.iterSpan = i.miter.Next()
	if !i.iterSpan.Valid() {
		i.valid = false
		return nil
	}
	return i.coalesceForward()
}

// Prev steps back to the previous span and returns it.
func (i *Iter) Prev() *CoalescedSpan {
	if i.dir == -1 && !i.iterSpan.Valid() {
		// If we were already going backward and the underlying iterator is
		// invalid, there is no previous item. Don't move the iterator, just
		// invalidate the iterator's position.
		i.valid = false
		return nil
	}
	i.dir = -1
	i.iterSpan = i.miter.Prev()
	if !i.iterSpan.Valid() {
		i.valid = false
		return nil
	}
	return i.coalesceBackward()
}

// Current returns the span at the iterator's current position, if any.
func (i *Iter) Current() *CoalescedSpan {
	if !i.valid {
		return nil
	}
	return &i.curr
}

// Valid returns true if the iterator is currently positioned over a span.
func (i *Iter) Valid() bool {
	return i.valid
}
