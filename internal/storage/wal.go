package storage

import (
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// WAL Operation types
type WALOpType uint8

const (
	WALOpAdd    WALOpType = 1
	WALOpDelete WALOpType = 2
	WALOpUpdate WALOpType = 3
)

// WALEntry represents a single operation in the write-ahead log.
type WALEntry struct {
	Timestamp  int64
	OpType     WALOpType
	Collection string
	Key        string
	VectorID   uint64
	Vector     []float32
	Keywords   []string
	Data       []byte // Primary data
}

// WAL provides write-ahead logging for atomic writes.
type WAL struct {
	filePath string
	file     *os.File
	encoder  *gob.Encoder
	mu       sync.Mutex
	seqNum   uint64
}

// NewWAL creates a new write-ahead log.
func NewWAL(filePath string) (*WAL, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open WAL file: %w", err)
	}

	return &WAL{
		filePath: filePath,
		file:     file,
		encoder:  gob.NewEncoder(file),
		seqNum:   0,
	}, nil
}

// LogAdd logs an add operation.
func (w *WAL) LogAdd(collection, key string, vectorID uint64, vector []float32, keywords []string, data []byte) error {
	return w.log(WALEntry{
		Timestamp:  time.Now().UnixNano(),
		OpType:     WALOpAdd,
		Collection: collection,
		Key:        key,
		VectorID:   vectorID,
		Vector:     vector,
		Keywords:   keywords,
		Data:       data,
	})
}

// LogDelete logs a delete operation.
func (w *WAL) LogDelete(collection, key string, vectorID uint64) error {
	return w.log(WALEntry{
		Timestamp:  time.Now().UnixNano(),
		OpType:     WALOpDelete,
		Collection: collection,
		Key:        key,
		VectorID:   vectorID,
	})
}

// log writes an entry to the WAL.
func (w *WAL) log(entry WALEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.seqNum++
	if err := w.encoder.Encode(entry); err != nil {
		return fmt.Errorf("failed to encode WAL entry: %w", err)
	}

	// Sync to ensure durability
	return w.file.Sync()
}

// Replay reads and returns all entries from the WAL.
func (w *WAL) Replay() ([]WALEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Seek to beginning
	if _, err := w.file.Seek(0, 0); err != nil {
		return nil, err
	}

	decoder := gob.NewDecoder(w.file)
	var entries []WALEntry

	for {
		var entry WALEntry
		if err := decoder.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return entries, nil // Return what we have on error
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// Checkpoint clears the WAL after successful commit.
func (w *WAL) Checkpoint() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Close current file
	if err := w.file.Close(); err != nil {
		return err
	}

	// Truncate the file
	file, err := os.Create(w.filePath)
	if err != nil {
		return err
	}

	w.file = file
	w.encoder = gob.NewEncoder(file)
	w.seqNum = 0

	return nil
}

// Close closes the WAL file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// Size returns the current size of the WAL file.
func (w *WAL) Size() (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	info, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// walHeader is used to identify and version the WAL file.
type walHeader struct {
	Magic   uint32 // Magic number to identify WAL files
	Version uint16 // WAL format version
}

const (
	walMagic   uint32 = 0x57414C00 // "WAL\0"
	walVersion uint16 = 1
)

// writeHeader writes the WAL header to a new file.
func writeWALHeader(file *os.File) error {
	header := walHeader{
		Magic:   walMagic,
		Version: walVersion,
	}
	return binary.Write(file, binary.BigEndian, header)
}

// readWALHeader reads and validates the WAL header.
func readWALHeader(file *os.File) error {
	var header walHeader
	if err := binary.Read(file, binary.BigEndian, &header); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // Empty file is OK
		}
		return err
	}

	if header.Magic != walMagic {
		return errors.New("invalid WAL file magic number")
	}
	if header.Version > walVersion {
		return fmt.Errorf("unsupported WAL version: %d", header.Version)
	}

	return nil
}
