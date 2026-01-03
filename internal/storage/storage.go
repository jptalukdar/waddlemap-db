package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"waddlemap/internal/types"

	"github.com/zeebo/blake3"
)

const PartitionCount = 16

type Manager struct {
	Config      *types.DBSchemaConfig
	Buckets     map[uint32]*Bucket
	mu          sync.RWMutex
	Compression bool
}

type Bucket struct {
	ID        uint32
	FilePath  string
	File      *os.File
	WriteLock sync.RWMutex
	Index     map[string][]int64 // Key -> List of Offsets in File
	IndexLock sync.RWMutex
}

// NewManager creates a new storage Manager instance with the provided database schema configuration.
// It initializes the data directory and creates/opens PartitionCount bucket files for data storage.
// Each bucket maintains its own file and in-memory index for key-value lookups.
// If a bucket's index file is corrupted or missing, it will be automatically rebuilt from the data file.
// Returns an error if directory creation fails, file operations fail, or bucket initialization fails.
func NewManager(cfg *types.DBSchemaConfig) (*Manager, error) {
	mgr := &Manager{
		Config:      cfg,
		Buckets:     make(map[uint32]*Bucket),
		Compression: true,
	}

	// Create data directory inside DataPath
	dataPath := filepath.Join(cfg.DataPath, "data")
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, err
	}

	for i := 0; i < PartitionCount; i++ {
		bucketID := uint32(i)
		fileName := fmt.Sprintf("waddle_shard_%03d.db", bucketID)
		filePath := filepath.Join(dataPath, fileName) // Use subdirectory

		f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return nil, err
		}

		b := &Bucket{
			ID:       bucketID,
			FilePath: filePath,
			File:     f,
			Index:    make(map[string][]int64),
		}

		// Load Index
		if err := b.loadIndex(); err != nil {
			log.Printf("Bucket %d: Rebuilding index... (Reason: %v)\n", bucketID, err)
			b.rebuildIndex()
			b.saveIndex()
		}

		mgr.Buckets[bucketID] = b
	}

	return mgr, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []string
	for _, b := range m.Buckets {
		if err := b.saveIndex(); err != nil {
			errs = append(errs, fmt.Sprintf("bucket %d save index: %v", b.ID, err))
		}
		if err := b.File.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("bucket %d close: %v", b.ID, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing manager: %s", strings.Join(errs, "; "))
	}
	return nil
}

// getBucketID computes a bucket ID for the given key using the BLAKE3 hash function.
// It hashes the key, extracts the first 4 bytes of the hash as a uint32 value in big-endian order,
// and returns the value modulo PartitionCount to ensure the bucket ID is within valid range.
func (m *Manager) getBucketID(key string) uint32 {
	h := blake3.New()
	h.Write([]byte(key))
	sum := h.Sum(nil)
	val := binary.BigEndian.Uint32(sum[:4])
	return val % PartitionCount
}

// ---------------- Operations ----------------

// Append adds a new entry to the storage for the given key and payload.
// The entry is appended to the end of the corresponding bucket file in the format:
// [KeyLen(4)][KeyBytes][PayloadLen(4)][PayloadBytes].
// It updates the in-memory index with the offset of the new entry.
// If SyncMode is set to "strict", the file is synced to disk after writing.
// Returns an error if any file or index operation fails.
func (m *Manager) Append(key string, payload []byte) error {
	// Security: Limit key and payload size to prevent abuse
	const maxKeyLen = 1024
	// const maxPayloadLen = 10 * 1024 * 1024 // 10MB

	if len(key) == 0 || len(key) > maxKeyLen {
		return fmt.Errorf("invalid key length")
	}
	// if len(payload) > maxPayloadLen {
	// 	return fmt.Errorf("payload too large")
	// }

	bucket := m.Buckets[m.getBucketID(key)]

	bucket.WriteLock.Lock()
	defer bucket.WriteLock.Unlock()

	offset, err := bucket.File.Seek(0, 2) // End // Append the data to the end of the file
	if err != nil {
		return err
	}

	// Format: [KeyLen(4 bytes - int32)][KeyBytes][PayloadLen(4 bytes - int32)][PayloadBytes]

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, int32(len(key))); err != nil {
		return err
	}
	if _, err := buf.Write([]byte(key)); err != nil {
		return err
	}

	compressedPayload := CompressBytes(payload)

	if len(compressedPayload) >= math.MaxInt32 {
		return fmt.Errorf("Payload size greater than MaxInt32 bytes after compression")
	}
	// Using int32 since we assume the data to be of smaller sizer. It can hold approx 2.14 GB
	if err := binary.Write(buf, binary.BigEndian, uint32(len(compressedPayload))); err != nil {
		return err
	}
	if _, err := buf.Write(compressedPayload); err != nil {
		return err
	}

	if _, err := bucket.File.Write(buf.Bytes()); err != nil {
		return err
	}

	// Update Index
	bucket.IndexLock.Lock()
	bucket.Index[key] = append(bucket.Index[key], offset)
	bucket.IndexLock.Unlock()

	if m.Config.SyncMode == "strict" {
		return bucket.File.Sync()
	}
	return nil
}

// BatchAppend adds multiple entries to the storage.
// It groups entries by bucket to minimize lock contention and file seeks.
func (m *Manager) BatchAppend(entries map[string][]byte) error {
	// 1. Group by Bucket to batch writes
	grouped := make(map[uint32][]struct {
		Key     string
		Payload []byte
	})

	for k, v := range entries {
		bid := m.getBucketID(k)
		grouped[bid] = append(grouped[bid], struct {
			Key     string
			Payload []byte
		}{k, v})
	}

	// 2. Process each bucket concurrently or sequentially
	// Using concurrency for speed
	var mu sync.Mutex
	var errs []string
	var wg sync.WaitGroup

	for bid, items := range grouped {
		wg.Add(1)
		go func(bucketID uint32, items []struct {
			Key     string
			Payload []byte
		}) {
			defer wg.Done()
			bucket := m.Buckets[bucketID]

			bucket.WriteLock.Lock()
			defer bucket.WriteLock.Unlock()

			// Seek once to end
			offset, err := bucket.File.Seek(0, 2)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("bucket %d seek: %v", bucketID, err))
				mu.Unlock()
				return
			}

			// Prepare Index updates
			newIndexEntries := make(map[string]int64)

			// Buffer writes? Or write individually?
			// Writing individually to file is okay if OS buffers, but we can buffer in memory.
			// Let's write individually for simplicity but under one lock.

			for _, item := range items {
				// START Format logic from Append()
				buf := new(bytes.Buffer)
				if err := binary.Write(buf, binary.BigEndian, int32(len(item.Key))); err != nil {
					// handle error
					continue
				}
				buf.Write([]byte(item.Key))

				compressedPayload := CompressBytes(item.Payload)
				if err := binary.Write(buf, binary.BigEndian, uint32(len(compressedPayload))); err != nil {
					continue
				}
				buf.Write(compressedPayload)

				n, err := bucket.File.Write(buf.Bytes())
				if err != nil {
					mu.Lock()
					errs = append(errs, fmt.Sprintf("bucket %d write key %s: %v", bucketID, item.Key, err))
					mu.Unlock()
					return // Stop usage of this bucket on error
				}
				// END Format

				newIndexEntries[item.Key] = offset
				offset += int64(n)
			}

			if m.Config.SyncMode == "strict" {
				bucket.File.Sync()
			}

			// Update Index in batch
			bucket.IndexLock.Lock()
			for k, off := range newIndexEntries {
				bucket.Index[k] = append(bucket.Index[k], off)
			}
			bucket.IndexLock.Unlock()

		}(bid, items)
	}
	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("batch append errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (m *Manager) Get(key string, index int) ([]byte, error) {
	bucket := m.Buckets[m.getBucketID(key)]

	bucket.IndexLock.RLock()
	offsets, exists := bucket.Index[key]
	bucket.IndexLock.RUnlock()

	if !exists || index >= len(offsets) || index < 0 {
		return nil, fmt.Errorf("index out of bounds or key not found")
	}

	offset := offsets[index]
	return bucket.readRecordAt(offset)
}

func (m *Manager) GetLength(key string) int {
	bucket := m.Buckets[m.getBucketID(key)]
	bucket.IndexLock.RLock()
	defer bucket.IndexLock.RUnlock()
	return len(bucket.Index[key])
}

func (m *Manager) Update(key string, index int, payload []byte) error {
	bucket := m.Buckets[m.getBucketID(key)]

	bucket.IndexLock.RLock()
	offsets, exists := bucket.Index[key]
	bucket.IndexLock.RUnlock()

	if !exists || index >= len(offsets) {
		return fmt.Errorf("item not found")
	}
	offset := offsets[index]

	// Check payload size constraint
	// For simplicity, we assume fixed payload size as per spec
	// Real impl would verify existing record size

	bucket.WriteLock.Lock()
	defer bucket.WriteLock.Unlock()

	// Skip Key Header to get to Payload
	// [KeyLen(4)][Key][PayloadLen(4)]...
	headerOffset := 4 + len(key) + 4
	payloadOffset := offset + int64(headerOffset)

	if _, err := bucket.File.WriteAt(payload, payloadOffset); err != nil {
		return err
	}
	return nil // No sync forced here unless strict
}

func (m *Manager) SearchGlobal(pattern []byte) ([][]byte, error) {
	var results [][]byte
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, b := range m.Buckets {
		wg.Add(1)
		go func(bucket *Bucket) {
			defer wg.Done()
			res := bucket.scan(pattern)
			if len(res) > 0 {
				mu.Lock()
				results = append(results, res...)
				mu.Unlock()
			}
		}(b)
	}
	wg.Wait()
	return results, nil
}

func (m *Manager) GetKeys() []string {
	var keys []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, b := range m.Buckets {
		wg.Add(1)
		go func(bucket *Bucket) {
			defer wg.Done()
			bucket.IndexLock.RLock()
			defer bucket.IndexLock.RUnlock()

			// Collect keys
			localKeys := make([]string, 0, len(bucket.Index))
			for k := range bucket.Index {
				localKeys = append(localKeys, k)
			}

			if len(localKeys) > 0 {
				mu.Lock()
				keys = append(keys, localKeys...)
				mu.Unlock()
			}
		}(b)
	}
	wg.Wait()
	return keys
}

func (m *Manager) GetAllValues(key string) ([][]byte, error) {
	bucket := m.Buckets[m.getBucketID(key)]

	bucket.IndexLock.RLock()
	offsets, exists := bucket.Index[key]
	bucket.IndexLock.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key not found")
	}

	results := make([][]byte, 0, len(offsets))
	// Optimize: Could be parallel read if payload is large
	for _, offset := range offsets {
		val, err := bucket.readRecordAt(offset)
		if err != nil {
			return nil, err
		}
		results = append(results, val)
	}
	return results, nil
}

func (m *Manager) Snapshot(name string) error {
	snapPath := filepath.Join(m.Config.DataPath, "snapshots", name)
	if err := os.MkdirAll(snapPath, 0755); err != nil {
		return err
	}

	for _, b := range m.Buckets {
		b.WriteLock.Lock() // Pause writes
		src, err := os.ReadFile(b.FilePath)
		if err != nil {
			b.WriteLock.Unlock()
			return err
		}
		b.WriteLock.Unlock() // Resume

		dstPath := filepath.Join(snapPath, filepath.Base(b.FilePath))
		if err := os.WriteFile(dstPath, src, 0644); err != nil {
			return err
		}
		// Also save index
		idxPath := dstPath + ".idx"
		// Not implementing index snapshot for brevity, easily rebuilt
		_ = idxPath
	}
	return nil
}

// ---------------- Helpers ----------------

func (b *Bucket) readRecordAt(offset int64) ([]byte, error) {
	// 1. Read Generic Header
	// We need KeyLen (4)
	header := make([]byte, 4)
	if _, err := b.File.ReadAt(header, offset); err != nil {
		return nil, err
	}
	keyLen := binary.BigEndian.Uint32(header)

	// 2. Read Rest
	// Offset + 4 + KeyLen -> PayloadLen
	// This is inefficient (multiple syscalls). Optimized: Read a larger chunk.

	// Let's read Header + Key + PayloadLen
	// But we don't know PayloadLen yet. So let's just seek past key.

	payloadLenOffset := offset + 4 + int64(keyLen)
	lenBuf := make([]byte, 4)
	if _, err := b.File.ReadAt(lenBuf, payloadLenOffset); err != nil {
		return nil, err
	}
	payloadLen := binary.BigEndian.Uint32(lenBuf)

	payload := make([]byte, payloadLen)
	if _, err := b.File.ReadAt(payload, payloadLenOffset+4); err != nil {
		return nil, err
	}

	payload, err := DecompressBytes(payload)
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func (b *Bucket) scan(pattern []byte) [][]byte {
	b.WriteLock.RLock()
	defer b.WriteLock.RUnlock()

	var matches [][]byte

	// Naive full scan. Note: mapped index helps finding records,
	// but global search needs to look at content.
	// Since we have memory offsets, we can iterate all records.

	b.IndexLock.RLock()
	defer b.IndexLock.RUnlock()

	for key, offsets := range b.Index {
		for _, offset := range offsets {
			val, err := b.readRecordAt(offset)
			if err == nil {
				if bytes.Contains(val, pattern) {
					matches = append(matches, val)
				}
			}
		}
		_ = key
	}
	return matches
}

// ---------------- Persistence ----------------

func (b *Bucket) indexFilePath() string {
	return b.FilePath + ".idx"
}

func (b *Bucket) saveIndex() error {
	b.IndexLock.RLock()
	defer b.IndexLock.RUnlock()

	f, err := os.Create(b.indexFilePath())
	if err != nil {
		return err
	}
	defer f.Close()

	enc := gob.NewEncoder(f)
	return enc.Encode(b.Index)
}

func (b *Bucket) loadIndex() error {
	f, err := os.Open(b.indexFilePath())
	if err != nil {
		return err
	}
	defer f.Close()

	b.IndexLock.Lock()
	defer b.IndexLock.Unlock()

	dec := gob.NewDecoder(f)
	return dec.Decode(&b.Index)
}

func (b *Bucket) rebuildIndex() {
	b.IndexLock.Lock()
	defer b.IndexLock.Unlock()

	// Reset
	b.Index = make(map[string][]int64)

	b.File.Seek(0, 0)
	offset, _ := b.File.Seek(0, 1)

	stat, _ := b.File.Stat()
	fileSize := stat.Size()

	var count int
	for offset < fileSize {
		// Read Key Len
		header := make([]byte, 4)
		if _, err := io.ReadFull(b.File, header); err != nil {
			break
		}
		keyLen := int64(binary.BigEndian.Uint32(header))

		// Read Key
		keyBuf := make([]byte, keyLen)
		if _, err := io.ReadFull(b.File, keyBuf); err != nil {
			break
		}
		key := string(keyBuf)

		if count < 10 && b.ID == 0 {
			log.Printf("Bucket %d: Record %d at %d - KeyLen: %d, Key: %s\n", b.ID, count, offset, keyLen, key)
		}

		// Read Payload Len
		if _, err := io.ReadFull(b.File, header); err != nil {
			break
		}
		payloadLen := int64(binary.BigEndian.Uint32(header))

		// Skip Payload
		if _, err := b.File.Seek(payloadLen, 1); err != nil {
			break
		}

		// Record Index
		b.Index[key] = append(b.Index[key], offset)
		count++

		if strings.Contains(key, "cycle") {
			log.Printf("Bucket %d: Found cycle key at offset %d\n", b.ID, offset)
		}

		// Next Offset
		offset, _ = b.File.Seek(0, 1)
	}
	log.Printf("Bucket %d: Rebuilt index with %d keys and %d records\n", b.ID, len(b.Index), count)
}
