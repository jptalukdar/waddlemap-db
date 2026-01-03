package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"waddlemap/internal/types"
)

// Collection represents a vector collection with all its indexes.
type Collection struct {
	Config       types.CollectionConfig
	HNSWIndex    *HNSWWrapper
	KeywordIndex *InvertedIndex
	DocMap       *ForwardIndex
	basePath     string
	mu           sync.RWMutex

	// In-Memory Indexes (Rebuilt on Load)
	KeyLengths map[string]uint32
	KeyIndex   map[string][]uint64 // Key -> List of VectorIDs
}

// CollectionManager manages all vector collections.
type CollectionManager struct {
	collections map[string]*Collection
	basePath    string // Base path for indexes directory
	mu          sync.RWMutex
}

// NewCollectionManager creates a new collection manager.
func NewCollectionManager(basePath string) (*CollectionManager, error) {
	indexesPath := filepath.Join(basePath, "indexes")
	if err := os.MkdirAll(indexesPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create indexes directory: %w", err)
	}

	cm := &CollectionManager{
		collections: make(map[string]*Collection),
		basePath:    indexesPath,
	}

	// Load existing collections
	if err := cm.loadExistingCollections(); err != nil {
		return nil, err
	}

	return cm, nil
}

// loadExistingCollections loads all existing collections from disk.
func (cm *CollectionManager) loadExistingCollections() error {
	entries, err := os.ReadDir(cm.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			collPath := filepath.Join(cm.basePath, entry.Name())
			meta, err := LoadCollectionMeta(collPath)
			if err != nil {
				// Skip invalid collection directories
				continue
			}

			coll, err := cm.loadCollection(meta)
			if err != nil {
				return fmt.Errorf("failed to load collection %s: %w", meta.Name, err)
			}
			cm.collections[meta.Name] = coll
		}
	}

	return nil
}

// loadCollection loads a single collection from disk.
func (cm *CollectionManager) loadCollection(meta *CollectionMeta) (*Collection, error) {
	collPath := filepath.Join(cm.basePath, meta.Name)

	// Create HNSW wrapper
	hnswPath := filepath.Join(collPath, "vectors.hnsw")
	hnsw, err := NewHNSWWrapper(meta.Dimensions, meta.Metric, hnswPath)
	if err != nil {
		return nil, err
	}

	// Load HNSW index using mmap
	if err := hnsw.Load(); err != nil {
		hnsw.Close()
		return nil, err
	}

	// Create keyword index
	kwPath := filepath.Join(collPath, "keywords.inv")
	kwIndex := NewInvertedIndex(kwPath)
	if err := kwIndex.Load(); err != nil {
		hnsw.Close()
		return nil, err
	}

	// Create forward index
	docMapPath := filepath.Join(collPath, "doc_map.bin")
	docMap := NewForwardIndex(docMapPath)
	if err := docMap.Load(); err != nil {
		hnsw.Close()
		return nil, err
	}

	coll := &Collection{
		Config: types.CollectionConfig{
			Name:       meta.Name,
			Dimensions: meta.Dimensions,
			Metric:     meta.Metric,
		},
		HNSWIndex:    hnsw,
		KeywordIndex: kwIndex,
		DocMap:       docMap,
		basePath:     collPath,
		KeyLengths:   make(map[string]uint32),
		KeyIndex:     make(map[string][]uint64),
	}

	// Rebuild In-Memory Indexes
	coll.rebuildMemoryIndexes()

	return coll, nil
}

// CreateCollection creates a new vector collection.
func (cm *CollectionManager) CreateCollection(name string, dimensions uint32, metric types.DistanceMetric) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if collection already exists
	if _, exists := cm.collections[name]; exists {
		return fmt.Errorf("collection %q already exists", name)
	}

	config := &types.CollectionConfig{
		Name:       name,
		Dimensions: dimensions,
		Metric:     metric,
	}
	if err := ValidateCollectionConfig(config); err != nil {
		return err
	}

	// Create collection directory
	collPath := filepath.Join(cm.basePath, name)
	if err := os.MkdirAll(collPath, 0755); err != nil {
		return fmt.Errorf("failed to create collection directory: %w", err)
	}

	// Save metadata
	meta := &CollectionMeta{
		Name:       name,
		Dimensions: dimensions,
		Metric:     metric,
	}
	if err := SaveCollectionMeta(collPath, meta); err != nil {
		os.RemoveAll(collPath)
		return fmt.Errorf("failed to save collection metadata: %w", err)
	}

	// Create HNSW wrapper
	hnswPath := filepath.Join(collPath, "vectors.hnsw")
	hnsw, err := NewHNSWWrapper(dimensions, metric, hnswPath)
	if err != nil {
		os.RemoveAll(collPath)
		return err
	}

	// Create keyword index
	kwPath := filepath.Join(collPath, "keywords.inv")
	kwIndex := NewInvertedIndex(kwPath)

	// Create forward index
	docMapPath := filepath.Join(collPath, "doc_map.bin")
	docMap := NewForwardIndex(docMapPath)

	collection := &Collection{
		Config:       *config,
		HNSWIndex:    hnsw,
		KeywordIndex: kwIndex,
		DocMap:       docMap,
		basePath:     collPath,
		KeyLengths:   make(map[string]uint32),
		KeyIndex:     make(map[string][]uint64),
	}

	cm.collections[name] = collection
	return nil
}

// DeleteCollection deletes a vector collection.
func (cm *CollectionManager) DeleteCollection(name string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	coll, exists := cm.collections[name]
	if !exists {
		return fmt.Errorf("collection %q not found", name)
	}

	// Close resources
	coll.Close()

	// Remove from map
	delete(cm.collections, name)

	// Delete directory
	return os.RemoveAll(coll.basePath)
}

// GetCollection returns a collection by name.
func (cm *CollectionManager) GetCollection(name string) (*Collection, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	coll, exists := cm.collections[name]
	if !exists {
		return nil, fmt.Errorf("collection %q not found", name)
	}
	return coll, nil
}

// ListCollections returns all collection configurations.
func (cm *CollectionManager) ListCollections() []types.CollectionConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	configs := make([]types.CollectionConfig, 0, len(cm.collections))
	for _, coll := range cm.collections {
		configs = append(configs, coll.Config)
	}
	return configs
}

// Close closes all collections and releases resources.
func (cm *CollectionManager) Close() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var lastErr error
	for _, coll := range cm.collections {
		if err := coll.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Collection methods

// Close saves and closes the collection.
func (c *Collection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error

	if err := c.HNSWIndex.Save(); err != nil {
		errs = append(errs, err)
	}
	if err := c.HNSWIndex.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := c.KeywordIndex.Save(); err != nil {
		errs = append(errs, err)
	}
	if err := c.DocMap.Save(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// AppendBlock adds a new block to the key.
func (c *Collection) AppendBlock(key string, block *types.BlockData) (uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Determine new index
	index := c.KeyLengths[key]

	// Get next vector ID
	vectorID := c.DocMap.GetNextVectorID()

	// Add to HNSW index (if vector present)
	if len(block.Vector) > 0 {
		if err := c.HNSWIndex.Add(vectorID, block.Vector); err != nil {
			return 0, fmt.Errorf("failed to add vector: %w", err)
		}
	}

	// Add to forward index (VectorID -> Key, Index)
	c.DocMap.Add(vectorID, key, index)

	// Add to keyword index
	if len(block.Keywords) > 0 {
		c.KeywordIndex.Add(block.Keywords, vectorID)
	}

	// Update Memory Indexes
	c.KeyLengths[key]++
	c.KeyIndex[key] = append(c.KeyIndex[key], vectorID)

	return index, nil
}

// Search performs vector similarity search.
func (c *Collection) Search(queryVector []float32, topK uint32, filter *types.SearchFilter) ([]types.SearchResultItem, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var bitset *BitSet

	// Apply keyword filter
	if filter != nil && len(filter.Keywords) > 0 {
		bitset = c.KeywordIndex.Search(filter.Keywords, filter.KeywordMode, filter.MaxDistance)
	}

	// Apply key filter
	if filter != nil && len(filter.Keys) > 0 {
		keyBitset := NewBitSet()
		for _, key := range filter.Keys {
			if vectorIDs, ok := c.KeyIndex[key]; ok {
				for _, id := range vectorIDs {
					keyBitset.Set(id)
				}
			}
		}
		if bitset == nil {
			bitset = keyBitset
		} else {
			bitset = bitset.Intersect(keyBitset) // Use generic Intersect? or bitset.Intersect returns BitSet
		}
	}

	// Perform HNSW search
	hnswResults, err := c.HNSWIndex.Search(queryVector, int(topK), bitset)
	if err != nil {
		return nil, err
	}

	// Convert results
	results := make([]types.SearchResultItem, 0, len(hnswResults))
	for _, hr := range hnswResults {
		loc, ok := c.DocMap.Get(hr.VectorID)
		if !ok {
			continue // Orphan
		}
		results = append(results, types.SearchResultItem{
			Key:      loc.Key,
			Index:    loc.Index,
			Distance: hr.Distance,
		})
	}

	return results, nil
}

// KeywordSearch performs keyword-only search.
func (c *Collection) KeywordSearch(keywords []string, mode string, maxDistance uint32) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	bitset := c.KeywordIndex.Search(keywords, mode, maxDistance)
	if bitset == nil || bitset.IsEmpty() {
		return nil, nil
	}

	// Collect Unique Keys
	uniqueKeys := make(map[string]struct{})
	for _, vectorID := range bitset.ToSlice() {
		if loc, ok := c.DocMap.Get(vectorID); ok {
			uniqueKeys[loc.Key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(uniqueKeys))
	for k := range uniqueKeys {
		keys = append(keys, k)
	}

	return keys, nil
}

// DeleteKey removes a key and all its blocks.
func (c *Collection) DeleteKey(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	vectorIDs, ok := c.KeyIndex[key]
	if !ok {
		return fmt.Errorf("key %q not found", key)
	}

	for _, id := range vectorIDs {
		c.HNSWIndex.Delete(id)
		// How to remove from KeywordIndex? Need to know keywords?
		// InvertedIndex supports Delete(keywords, id).
		// We don't track keywords per id here.
		// So KeywordIndex might just keep stale IDs? Or cleanup on rebuild?
		// Alternatively, KeywordIndex.DeleteDoc(id)? (Not implemented).
		// For now, accept stale IDs in KeywordIndex (Search validates against DocMap/ForwardIndex if we check existence).
		// Wait, Search uses BitSet from Keywords. If we fetch result from HNSW/BitSet, we check DocMap.
		// If DocMap.Delete(id) is called, Get(id) returns false.
		// So stale keywords return IDs that are filtered out at end.
		// Correct.
		c.DocMap.Delete(id)
	}

	delete(c.KeyLengths, key)
	delete(c.KeyIndex, key)
	return nil
}

// GetKeyLength returns the number of blocks for a key.
func (c *Collection) GetKeyLength(key string) (uint32, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if l, ok := c.KeyLengths[key]; ok {
		return l, nil
	}
	return 0, fmt.Errorf("key %q not found", key)
}

// GetBlockVectorID returns the VectorID for a specific block.
func (c *Collection) GetBlockVectorID(key string, index uint32) (uint64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	vectorIDs, ok := c.KeyIndex[key]
	if !ok {
		return 0, fmt.Errorf("key %q not found", key)
	}

	// We need to find which ID corresponds to this Index.
	// KeyIndex is just a list of IDs. Order not guaranteed by append?
	// Actually AppendBlock appends sequentially.
	// So KeyIndex[index] might be correct if we assume Append order.
	// But Delete/Replace operations might mess this up?
	// Replacing block keeps index same.
	// So just check DocMap?
	// Iterate IDs for this key and check index.
	for _, id := range vectorIDs {
		if loc, ok := c.DocMap.Get(id); ok {
			if loc.Index == index {
				return id, nil
			}
		}
	}
	return 0, fmt.Errorf("block %d not found for key %q", index, key)
}

// ListKeys returns all keys in the collection.
func (c *Collection) ListKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, 0, len(c.KeyLengths))
	for k := range c.KeyLengths {
		keys = append(keys, k)
	}
	return keys
}

// ContainsKey checks if a key exists.
func (c *Collection) ContainsKey(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.KeyLengths[key]
	return ok
}

// Save persists all indexes.
func (c *Collection) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Implementation matches existing Save
	var errs []error
	if err := c.HNSWIndex.Save(); err != nil {
		errs = append(errs, err)
	}
	if err := c.KeywordIndex.Save(); err != nil {
		errs = append(errs, err)
	}
	if err := c.DocMap.Save(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// FlushHNSW saves only the HNSW index to disk.
// Use this after batch operations to minimize I/O overhead.
func (c *Collection) FlushHNSW() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.HNSWIndex.Save()
}

// rebuildMemoryIndexes rebuilds KeyLengths and KeyIndex from DocMap.
func (c *Collection) rebuildMemoryIndexes() {
	// Access DocMap directly (already locked by caller or initialized)
	// Iterate raw map
	c.DocMap.mu.RLock()
	defer c.DocMap.mu.RUnlock()

	for id, loc := range c.DocMap.mapping {
		// Update Key Index
		c.KeyIndex[loc.Key] = append(c.KeyIndex[loc.Key], id)

		// Update Length -> Max Index + 1
		if loc.Index >= c.KeyLengths[loc.Key] {
			c.KeyLengths[loc.Key] = loc.Index + 1
		}
	}
}

// Count returns the number of vectors in the collection.
func (c *Collection) Count() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.HNSWIndex.Count()
}

// GetVectorByID retrieves a vector by its ID.
func (c *Collection) GetVectorByID(id uint64) ([]float32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, ok := c.HNSWIndex.nodes[id]
	if !ok {
		return nil, false
	}
	return node.Vector, true
}
