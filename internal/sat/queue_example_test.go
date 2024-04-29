package sat

import "fmt"

func ExampleNewQueue() {
	q := NewQueue[int](2)

	fmt.Println(q)

	q.Push(1)
	q.Push(2)

	fmt.Println(q)

	// Output:
	// Queue[]
	// Queue[1 2]
}

func ExampleQueue_IsEmpty() {
	q := NewQueue[int](1)

	fmt.Println(q.IsEmpty())
	q.Push(1)
	fmt.Println(q.IsEmpty())

	// Output:
	// true
	// false
}

func ExampleQueue_Size() {
	q := NewQueue[int](1)

	fmt.Println(q.Size())
	q.Push(1)
	q.Push(2)
	q.Push(3)
	q.Push(4)
	fmt.Println(q.Size())

	// Output:
	// 0
	// 4
}

func ExampleQueue_Clear() {
	q := NewQueue[int](1)

	q.Push(1)
	q.Push(2)
	q.Push(3)
	q.Push(4)
	q.Clear()

	fmt.Println(q)

	// Output:
	// Queue[]
}

func ExampleQueue_Push() {
	q := NewQueue[int](1)

	q.Push(1)
	q.Push(2)
	q.Push(3)
	q.Push(4)

	fmt.Println(q)

	// Output:
	// Queue[1 2 3 4]
}

func ExampleQueue_Pop() {
	q := NewQueue[int](1)

	q.Push(1)
	q.Push(2)
	q.Push(3)
	q.Push(4)

	q.Pop()
	q.Pop()

	fmt.Println(q)

	// Output:
	// Queue[3 4]
}
