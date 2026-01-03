package storage

import (
	"encoding/binary"
	"encoding/gob"
	"errors"
	"os"
	"sync"
)

// DocLocation represents a block within a key.
type DocLocation struct {
	Key   string
	Index uint32
}

// ForwardIndex provides O(1) VectorID → (Key, Index) lookup.
// This corresponds to the doc_map.bin file in the spec.
type ForwardIndex struct {
	mapping  map[uint64]DocLocation
	filePath string
	mu       sync.RWMutex
}

// NewForwardIndex creates a new forward index.
func NewForwardIndex(filePath string) *ForwardIndex {
	return &ForwardIndex{
		mapping:  make(map[uint64]DocLocation),
		filePath: filePath,
	}
}

// Add adds a VectorID → (Key, Index) mapping.
func (fi *ForwardIndex) Add(vectorID uint64, key string, index uint32) {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	fi.mapping[vectorID] = DocLocation{Key: key, Index: index}
}

// Get retrieves a document location by VectorID.
func (fi *ForwardIndex) Get(vectorID uint64) (DocLocation, bool) {
	fi.mu.RLock()
	defer fi.mu.RUnlock()
	loc, ok := fi.mapping[vectorID]
	return loc, ok
}

// Delete removes a VectorID mapping.
func (fi *ForwardIndex) Delete(vectorID uint64) {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	delete(fi.mapping, vectorID)
}

// Count returns the number of entries in the forward index.
func (fi *ForwardIndex) Count() int {
	fi.mu.RLock()
	defer fi.mu.RUnlock()
	return len(fi.mapping)
}

// Save persists the forward index to disk using GOB.
func (fi *ForwardIndex) Save() error {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	file, err := os.Create(fi.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(fi.mapping)
}

// Load reads the forward index from disk.
func (fi *ForwardIndex) Load() error {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	file, err := os.Open(fi.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			fi.mapping = make(map[uint64]DocLocation)
			return nil
		}
		return err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	return decoder.Decode(&fi.mapping)
}

// GetNextVectorID returns and reserves the next available vector ID.
func (fi *ForwardIndex) GetNextVectorID() uint64 {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	var maxID uint64 = 0
	for id := range fi.mapping {
		if id > maxID {
			maxID = id
		}
	}
	return maxID + 1
}

// VectorIDToBytes converts a VectorID to bytes for storage.
func VectorIDToBytes(id uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, id)
	return buf
}

// BytesToVectorID converts bytes back to a VectorID.
func BytesToVectorID(data []byte) (uint64, error) {
	if len(data) != 8 {
		return 0, errors.New("invalid vector ID data length")
	}
	return binary.BigEndian.Uint64(data), nil
}
