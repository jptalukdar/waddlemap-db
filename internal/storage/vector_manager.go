package storage

import (
	"fmt"
	"path/filepath"
	"sync"

	"waddlemap/internal/types"
)

// VectorManager extends Manager with vector store capabilities.
type VectorManager struct {
	*Manager
	collections *CollectionManager
	wal         *WAL
	repair      *RepairManager
	mu          sync.RWMutex
}

// NewVectorManager creates a new vector-enabled storage manager.
func NewVectorManager(cfg *types.DBSchemaConfig) (*VectorManager, error) {
	// Create base manager
	baseMgr, err := NewManager(cfg)
	if err != nil {
		return nil, err
	}

	// Create collection manager
	collMgr, err := NewCollectionManager(cfg.DataPath)
	if err != nil {
		baseMgr.Close()
		return nil, err
	}

	// Create WAL
	walPath := filepath.Join(cfg.DataPath, "vector.wal")
	wal, err := NewWAL(walPath)
	if err != nil {
		collMgr.Close()
		baseMgr.Close()
		return nil, err
	}

	vm := &VectorManager{
		Manager:     baseMgr,
		collections: collMgr,
		wal:         wal,
	}

	// Create repair manager
	vm.repair = NewRepairManager(collMgr)

	// Recover from WAL
	if err := vm.recoverFromWAL(walPath); err != nil {
		fmt.Printf("Warning: WAL recovery failed: %v\n", err)
	}

	return vm, nil
}

// recoverFromWAL replays WAL logs.
func (vm *VectorManager) recoverFromWAL(walPath string) error {
	entries, err := vm.wal.Replay()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		switch entry.OpType {
		case WALOpAdd:
			// Map legacy Add to AppendBlock
			block := &types.BlockData{
				Primary:  string(entry.Data),
				Vector:   entry.Vector,
				Keywords: entry.Keywords,
			}
			_, err := vm.AppendBlock(entry.Collection, entry.Key, block)
			if err != nil {
				return err
			}

		case WALOpDelete:
			if err := vm.DeleteKey(entry.Collection, entry.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateCollection creates a new vector collection.
func (vm *VectorManager) CreateCollection(name string, dimensions uint32, metric types.DistanceMetric) error {
	return vm.collections.CreateCollection(name, dimensions, metric)
}

// DeleteCollection deletes a vector collection.
func (vm *VectorManager) DeleteCollection(name string) error {
	return vm.collections.DeleteCollection(name)
}

// ListCollections returns all collection configurations.
func (vm *VectorManager) ListCollections() []types.CollectionConfig {
	return vm.collections.ListCollections()
}

// GetCollection returns a collection by name.
func (vm *VectorManager) GetCollection(name string) (*Collection, error) {
	return vm.collections.GetCollection(name)
}

// AppendBlock appends a block to a key.
func (vm *VectorManager) AppendBlock(collection, key string, block *types.BlockData) (uint32, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return 0, err
	}

	if err := vm.wal.LogAdd(collection, key, 0, block.Vector, block.Keywords, []byte(block.Primary)); err != nil {
		return 0, fmt.Errorf("WAL logging failed: %w", err)
	}

	index, err := coll.AppendBlock(key, block)
	if err != nil {
		return 0, err
	}

	vectorID, err := coll.GetBlockVectorID(key, index)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve vector ID after append: %w", err)
	}

	// Serialize Entry
	entry := &Entry{
		Key:           []byte(key),
		Keywords:      block.Keywords,
		PrimaryData:   []byte(block.Primary),
		SecondaryData: VectorIDToBytes(vectorID),
		Flags:         types.EntryFlags{},
	}
	if len(block.Vector) > 0 {
		entry.Flags.DataType = types.DataTypeVector
	}

	encoded, err := EncodeEntry(entry)
	if err != nil {
		return 0, fmt.Errorf("failed to encode entry: %w", err)
	}

	if err := vm.Manager.Append(key, encoded); err != nil {
		return index, fmt.Errorf("storage append failed: %w", err)
	}

	return index, nil
}

// BatchAppendBlocks appends multiple blocks effectively.
func (vm *VectorManager) BatchAppendBlocks(collection string, keys []string, blocks []*types.BlockData) ([]bool, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return nil, err
	}

	successes := make([]bool, len(keys))
	batchEntries := make(map[string][]byte)

	// Phase 1: In-Memory updates & preparation
	for i, key := range keys {
		block := blocks[i]

		// WAL (Individual for now, TODO: BatchWAL)
		if err := vm.wal.LogAdd(collection, key, 0, block.Vector, block.Keywords, []byte(block.Primary)); err != nil {
			// Log error but continue? Or fail all?
			// If WAL fails, we shouldn't proceed for this item.
			continue
		}

		index, err := coll.AppendBlock(key, block)
		if err != nil {
			continue
		}

		vectorID, err := coll.GetBlockVectorID(key, index)
		if err != nil {
			continue
		}

		// Serialize Entry
		entry := &Entry{
			Key:           []byte(key),
			Keywords:      block.Keywords,
			PrimaryData:   []byte(block.Primary),
			SecondaryData: VectorIDToBytes(vectorID),
			Flags:         types.EntryFlags{},
		}
		if len(block.Vector) > 0 {
			entry.Flags.DataType = types.DataTypeVector
		}

		encoded, err := EncodeEntry(entry)
		if err != nil {
			continue
		}

		batchEntries[key] = encoded
		successes[i] = true
	}

	// Phase 2: Batch Storage Write
	if len(batchEntries) > 0 {
		if err := vm.Manager.BatchAppend(batchEntries); err != nil {
			// If storage fails, technically we are in inconsistent state (memory has it, disk doesn't).
			// Robustness would require rollback or repair.
			// For this implementation, we return error.
			return successes, fmt.Errorf("batch storage write failed: %w", err)
		}
	}

	return successes, nil
}

// GetBlock retrieves a specific block.
func (vm *VectorManager) GetBlock(collection, key string, index uint32) (*types.BlockData, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return nil, err
	}

	if exists := coll.ContainsKey(key); !exists {
		return nil, fmt.Errorf("key %q not found", key)
	}

	payload, err := vm.Manager.Get(key, int(index))
	if err != nil {
		return nil, err
	}

	entry, err := DecodeEntry(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode entry: %w", err)
	}

	block := &types.BlockData{
		Primary:  string(entry.PrimaryData),
		Keywords: entry.Keywords,
	}

	if len(entry.SecondaryData) == 8 {
		vectorID, _ := BytesToVectorID(entry.SecondaryData)
		if vec, ok := coll.GetVectorByID(vectorID); ok {
			block.Vector = vec
		}
	}

	return block, nil
}

// GetVector retrieves just the vector for a block.
func (vm *VectorManager) GetVector(collection, key string, index uint32) ([]float32, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return nil, err
	}

	vectorID, err := coll.GetBlockVectorID(key, index)
	if err != nil {
		return nil, err
	}

	vec, ok := coll.GetVectorByID(vectorID)
	if !ok {
		return nil, fmt.Errorf("vector data missing for ID %d", vectorID)
	}
	return vec, nil
}

// GetKeyLength returns the number of blocks.
func (vm *VectorManager) GetKeyLength(collection, key string) (uint32, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return 0, err
	}
	return coll.GetKeyLength(key)
}

// GetKey retrieves all blocks for a key.
func (vm *VectorManager) GetKey(collection, key string) ([]types.BlockData, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return nil, err
	}

	if exists := coll.ContainsKey(key); !exists {
		return nil, fmt.Errorf("key %q not found", key)
	}

	payloads, err := vm.Manager.GetAllValues(key)
	if err != nil {
		return nil, err
	}

	blocks := make([]types.BlockData, 0, len(payloads))
	for _, p := range payloads {
		entry, err := DecodeEntry(p)
		if err != nil {
			continue // Skip malformed
		}

		block := types.BlockData{
			Primary:  string(entry.PrimaryData),
			Keywords: entry.Keywords,
		}

		if len(entry.SecondaryData) == 8 {
			vid, _ := BytesToVectorID(entry.SecondaryData)
			if vec, ok := coll.GetVectorByID(vid); ok {
				block.Vector = vec
			}
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

// DeleteKey deletes a key and all blocks.
func (vm *VectorManager) DeleteKey(collection, key string) error {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return err
	}

	if err := vm.wal.LogDelete(collection, key, 0); err != nil {
		return err
	}

	if err := coll.DeleteKey(key); err != nil {
		return err
	}

	// Note: Primary data in Manager not deleted, but index cleared in Collection.
	return nil
}

// ListKeys lists keys.
func (vm *VectorManager) ListKeys(collection string) ([]string, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return nil, err
	}
	return coll.ListKeys(), nil
}

// ContainsKey checks existence.
func (vm *VectorManager) ContainsKey(collection, key string) (bool, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return false, err
	}
	return coll.ContainsKey(key), nil
}

// UpdateBlock updates a block.
func (vm *VectorManager) UpdateBlock(collection, key string, index uint32, block *types.BlockData) error {
	// Stub
	return fmt.Errorf("not implemented")
}

func (vm *VectorManager) ReplaceBlock(collection, key string, index uint32, block *types.BlockData) error {
	return vm.UpdateBlock(collection, key, index, block)
}

// Search performs search.
func (vm *VectorManager) Search(collection string, query []float32, topK uint32, mode string, keywords []string) ([]types.SearchResultItem, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return nil, err
	}

	filter := &types.SearchFilter{
		Keywords:    keywords,
		KeywordMode: "exact",
	}
	if mode != "" {
		filter.KeywordMode = mode
	}

	results, err := coll.Search(query, topK, filter)
	if err != nil {
		return nil, err
	}

	for i := range results {
		block, err := vm.GetBlock(collection, results[i].Key, results[i].Index)
		if err == nil {
			results[i].Block = block
		}
	}

	return results, nil
}

func (vm *VectorManager) SearchMLT(collection, key string, index uint32, topK uint32) ([]types.SearchResultItem, error) {
	vec, err := vm.GetVector(collection, key, index)
	if err != nil {
		return nil, fmt.Errorf("failed to get query vector: %w", err)
	}
	return vm.Search(collection, vec, topK, "global", nil)
}

func (vm *VectorManager) SearchInKey(collection, key string, query []float32, topK uint32) ([]types.SearchResultItem, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return nil, err
	}

	filter := &types.SearchFilter{
		Keys: []string{key},
	}

	results, err := coll.Search(query, topK, filter)
	if err != nil {
		return nil, err
	}

	for i := range results {
		block, err := vm.GetBlock(collection, results[i].Key, results[i].Index)
		if err == nil {
			results[i].Block = block
		}
	}
	return results, nil
}

// KeywordSearch performs keyword-only search.
func (vm *VectorManager) KeywordSearch(collection string, keywords []string, mode string, maxDistance uint32) ([]string, error) {
	coll, err := vm.collections.GetCollection(collection)
	if err != nil {
		return nil, err
	}
	return coll.KeywordSearch(keywords, mode, maxDistance)
}

func (vm *VectorManager) SnapshotCollection(collection string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (vm *VectorManager) CompactCollection(collection string) error {
	return fmt.Errorf("not implemented")
}

// Checkpoint clears the WAL.
func (vm *VectorManager) Checkpoint() error {
	for _, config := range vm.collections.ListCollections() {
		coll, err := vm.collections.GetCollection(config.Name)
		if err == nil {
			coll.Save()
		}
	}
	return vm.wal.Checkpoint()
}

// Close closes everything.
func (vm *VectorManager) Close() error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.Checkpoint()
	vm.wal.Close()
	vm.collections.Close()
	vm.Manager.Close()
	return nil
}
