package trimslice

import (
	"fmt"
	"slices"
	"testing"
)

var lens = []int{
	0, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233, 377, 610, 987, 1597, 2584, 4181, 6765, 10946, 17711, 28657, 46368, 75025, 121393, 196418,
}

func TestTrimSlice(t *testing.T) {
	t.Parallel()

	for _, l := range lens {
		t.Run(fmt.Sprintf("len %d", l), func(t *testing.T) {
			t.Parallel()

			var s []int
			for i := range l {
				s = append(s, i)
			}

			got := TrimSlice(s)
			if len(got) != len(s) {
				t.Fatalf("len = %d, want %d", len(got), len(s))
			}
			if cap(got) != len(s) {
				t.Fatalf("cap = %d, want %d", cap(got), len(s))
			}
			if !slices.Equal(got, s) {
				t.Fatal("slices not equal")
			}
		})
	}
}
