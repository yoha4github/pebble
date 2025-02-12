// Copyright 2021 The LevelDB-Go and Pebble Authors. All rights reserved. Use
// of this source code is governed by a BSD-style license that can be found in
// the LICENSE file.

package rangekey

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/cockroachdb/pebble/internal/base"
	"github.com/cockroachdb/pebble/internal/datadriven"
	"github.com/cockroachdb/pebble/internal/keyspan"
	"github.com/cockroachdb/pebble/internal/testkeys"
	"github.com/stretchr/testify/require"
)

func TestHasSets(t *testing.T) {
	testCases := map[*CoalescedSpan]bool{
		{
			Items: []SuffixItem{{Suffix: []byte("foo")}},
		}: true,
		{
			Items: []SuffixItem{{Unset: true, Suffix: []byte("foo")}},
		}: false,
		{
			Items: []SuffixItem{
				{Unset: true, Suffix: []byte("foo")},
				{Unset: true, Suffix: []byte("foo2")},
				{Unset: true, Suffix: []byte("foo3")},
			},
		}: false,
		{
			Items: []SuffixItem{
				{Unset: true, Suffix: []byte("foo")},
				{Unset: false, Suffix: []byte("foo2")},
				{Unset: true, Suffix: []byte("foo3")},
			},
		}: true,
	}
	for s, want := range testCases {
		got := s.HasSets()
		if got != want {
			var buf bytes.Buffer
			formatRangeKeySpan(&buf, s)
			t.Errorf("%s.HasSets() = %t, want %t", buf.String(), got, want)
		}
	}
}

func TestSmallestSetSuffix(t *testing.T) {
	testCases := []struct {
		span      *CoalescedSpan
		threshold []byte
		want      []byte
	}{
		{
			span:      &CoalescedSpan{Items: []SuffixItem{{Suffix: []byte("foo")}}},
			threshold: []byte("apple"),
			want:      []byte("foo"),
		},
		{
			span:      &CoalescedSpan{Items: []SuffixItem{{Suffix: []byte("foo")}}},
			threshold: []byte("xyz"),
			want:      nil,
		},
		{
			span:      &CoalescedSpan{Items: []SuffixItem{{Suffix: []byte("foo")}}},
			threshold: []byte("foo"),
			want:      []byte("foo"),
		},
		{
			span: &CoalescedSpan{Items: []SuffixItem{
				{Suffix: []byte("ace")},
				{Suffix: []byte("bar"), Unset: true},
				{Suffix: []byte("foo")},
			}},
			threshold: []byte("apple"),
			want:      []byte("foo"),
		},
	}
	for _, tc := range testCases {
		got := tc.span.SmallestSetSuffix(base.DefaultComparer.Compare, tc.threshold)
		if !bytes.Equal(got, tc.want) {
			var buf bytes.Buffer
			formatRangeKeySpan(&buf, tc.span)
			t.Errorf("%s.SmallestSetSuffix(%q) = %q, want %q",
				buf.String(), tc.threshold, got, tc.want)
		}
	}
}

func TestCoalesce(t *testing.T) {
	var buf bytes.Buffer
	cmp := testkeys.Comparer.Compare

	datadriven.RunTest(t, "testdata/coalesce", func(td *datadriven.TestData) string {
		switch td.Cmd {
		case "coalesce":
			buf.Reset()
			s, err := Coalesce(cmp, keyspan.ParseSpan(td.Input))
			if err != nil {
				return err.Error()
			}
			formatRangeKeySpan(&buf, &s)
			return buf.String()
		default:
			return fmt.Sprintf("unrecognized command %q", td.Cmd)
		}
	})
}

func TestIter(t *testing.T) {
	cmp := testkeys.Comparer.Compare
	var iter Iter
	var buf bytes.Buffer

	datadriven.RunTest(t, "testdata/iter", func(td *datadriven.TestData) string {
		buf.Reset()
		switch td.Cmd {
		case "define":
			visibleSeqNum := base.InternalKeySeqNumMax
			for _, arg := range td.CmdArgs {
				if arg.Key == "visible-seq-num" {
					var err error
					visibleSeqNum, err = strconv.ParseUint(arg.Vals[0], 10, 64)
					require.NoError(t, err)
				}
			}

			var spans []keyspan.Span
			lines := strings.Split(strings.TrimSpace(td.Input), "\n")
			for _, line := range lines {
				spans = append(spans, keyspan.ParseSpan(line))
			}
			iter.Init(cmp, testkeys.Comparer.FormatKey, visibleSeqNum, keyspan.NewIter(cmp, spans))
			return "OK"
		case "iter":
			buf.Reset()
			lines := strings.Split(strings.TrimSpace(td.Input), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				i := strings.IndexByte(line, ' ')
				iterCmd := line
				if i > 0 {
					iterCmd = string(line[:i])
				}
				switch iterCmd {
				case "first":
					formatRangeKeySpan(&buf, iter.First())
				case "last":
					formatRangeKeySpan(&buf, iter.Last())
				case "next":
					formatRangeKeySpan(&buf, iter.Next())
				case "prev":
					formatRangeKeySpan(&buf, iter.Prev())
				case "seek-ge":
					formatRangeKeySpan(&buf, iter.SeekGE([]byte(strings.TrimSpace(line[i:]))))
				case "seek-lt":
					formatRangeKeySpan(&buf, iter.SeekLT([]byte(strings.TrimSpace(line[i:]))))
				default:
					return fmt.Sprintf("unrecognized iter command %q", iterCmd)
				}
				require.NoError(t, iter.Error())
				if buf.Len() > 0 {
					fmt.Fprintln(&buf)
				}
			}
			return buf.String()
		default:
			return fmt.Sprintf("unrecognized command %q", td.Cmd)
		}
	})
}

func formatRangeKeySpan(w io.Writer, rks *CoalescedSpan) {
	if rks == nil {
		fmt.Fprintf(w, ".")
		return
	}
	fmt.Fprintf(w, "●   [%s, %s)#%d", rks.Start, rks.End, rks.LargestSeqNum)
	if rks.Delete {
		fmt.Fprintf(w, " (DEL)")
	}
	formatRangeKeyItems(w, rks.Items)
}

func formatRangeKeyItems(w io.Writer, items []SuffixItem) {
	for i := range items {
		fmt.Fprintln(w)
		if i != len(items)-1 {
			fmt.Fprint(w, "├──")
		} else {
			fmt.Fprint(w, "└──")
		}
		fmt.Fprintf(w, " %s", items[i].Suffix)
		if items[i].Unset {
			fmt.Fprint(w, " unset")
		} else {
			fmt.Fprintf(w, " : %s", items[i].Value)
		}
	}
}
