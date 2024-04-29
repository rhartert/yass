package sat

import (
	"reflect"
	"testing"
)

func TestQueue_Push_WithResizeAndRotation(t *testing.T) {
	q := &Queue[int]{
		ring:  []int{3, 4, 1, 2},
		start: 2,
		end:   2,
		size:  4,
		mask:  0b11,
	}
	want := &Queue[int]{
		ring:  []int{1, 2, 3, 4, 5, 0, 0, 0},
		start: 0,
		end:   5,
		size:  5,
		mask:  0b111,
	}

	q.Push(5)

	if !reflect.DeepEqual(want, q) {
		t.Errorf("Mismatch: want %#v, got %#v", want, q)
	}
}
