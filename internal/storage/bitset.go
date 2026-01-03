package storage

import (
	"sort"
	"sync"
)

// BitSet is a simple bit set implementation for efficient set operations.
// Used for filtering VectorIDs during keyword and vector search.
type BitSet struct {
	bits map[uint64]struct{}
	mu   sync.RWMutex
}

// NewBitSet creates a new empty BitSet.
func NewBitSet() *BitSet {
	return &BitSet{
		bits: make(map[uint64]struct{}),
	}
}

// NewBitSetFromSlice creates a BitSet from a slice of uint64 values.
func NewBitSetFromSlice(values []uint64) *BitSet {
	bs := NewBitSet()
	for _, v := range values {
		bs.bits[v] = struct{}{}
	}
	return bs
}

// Set adds a value to the BitSet.
func (bs *BitSet) Set(value uint64) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.bits[value] = struct{}{}
}

// Unset removes a value from the BitSet.
func (bs *BitSet) Unset(value uint64) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	delete(bs.bits, value)
}

// Contains checks if a value is in the BitSet.
func (bs *BitSet) Contains(value uint64) bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	_, ok := bs.bits[value]
	return ok
}

// Count returns the number of values in the BitSet.
func (bs *BitSet) Count() int {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return len(bs.bits)
}

// IsEmpty returns true if the BitSet is empty.
func (bs *BitSet) IsEmpty() bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return len(bs.bits) == 0
}

// ToSlice returns all values in the BitSet as a sorted slice.
func (bs *BitSet) ToSlice() []uint64 {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	result := make([]uint64, 0, len(bs.bits))
	for v := range bs.bits {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

// Intersect returns a new BitSet containing only values present in both sets.
func (bs *BitSet) Intersect(other *BitSet) *BitSet {
	if bs == nil || other == nil {
		return NewBitSet()
	}

	bs.mu.RLock()
	other.mu.RLock()
	defer bs.mu.RUnlock()
	defer other.mu.RUnlock()

	result := NewBitSet()

	// Iterate over the smaller set for efficiency
	smaller, larger := bs.bits, other.bits
	if len(bs.bits) > len(other.bits) {
		smaller, larger = other.bits, bs.bits
	}

	for v := range smaller {
		if _, ok := larger[v]; ok {
			result.bits[v] = struct{}{}
		}
	}

	return result
}

// Union returns a new BitSet containing all values from both sets.
func (bs *BitSet) Union(other *BitSet) *BitSet {
	if bs == nil {
		return other
	}
	if other == nil {
		return bs
	}

	bs.mu.RLock()
	other.mu.RLock()
	defer bs.mu.RUnlock()
	defer other.mu.RUnlock()

	result := NewBitSet()
	for v := range bs.bits {
		result.bits[v] = struct{}{}
	}
	for v := range other.bits {
		result.bits[v] = struct{}{}
	}

	return result
}

// Difference returns a new BitSet with values in bs but not in other.
func (bs *BitSet) Difference(other *BitSet) *BitSet {
	if bs == nil {
		return NewBitSet()
	}
	if other == nil {
		return bs
	}

	bs.mu.RLock()
	other.mu.RLock()
	defer bs.mu.RUnlock()
	defer other.mu.RUnlock()

	result := NewBitSet()
	for v := range bs.bits {
		if _, ok := other.bits[v]; !ok {
			result.bits[v] = struct{}{}
		}
	}

	return result
}

// Clone returns a copy of the BitSet.
func (bs *BitSet) Clone() *BitSet {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	result := NewBitSet()
	for v := range bs.bits {
		result.bits[v] = struct{}{}
	}
	return result
}
