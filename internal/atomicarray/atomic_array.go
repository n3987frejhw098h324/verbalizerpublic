package atomicarray

import "sync/atomic"

type AtomicArray[T any] struct {
	arr atomic.Pointer[[]T]
}

func New[T any](arr *[]T) *AtomicArray[T] {
	s := &AtomicArray[T]{}
	s.arr.Store(arr)
	return s
}

func (s *AtomicArray[T]) Load() []T {
	return *s.arr.Load()
}

func (s *AtomicArray[T]) Update(updateFunc func(arr []T) []T) {
	for {
		arr := s.arr.Load()

		res := updateFunc(*arr)
		if res == nil {
			return
		}

		if s.arr.CompareAndSwap(arr, &res) {
			return
		}
	}
}

func MoveToFront[T comparable](arr *AtomicArray[T], value T) {
	arr.Update(func(currentArray []T) []T {
		if currentArray[0] == value {
			return nil
		}

		for i, v := range currentArray {
			if v != value {
				continue
			}
			if i == 1 {
				currentArray[0], currentArray[1] = currentArray[1], currentArray[0]
				return currentArray
			}

			copy(currentArray[1:i+1], currentArray[0:i])
			currentArray[0] = value
			return currentArray
		}

		return nil
	})
}
