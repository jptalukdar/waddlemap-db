package storage

import (
	"container/heap"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"waddlemap/internal/types"
)

// HNSW binary format constants
const (
	hnswMagic      = "HNSWV001"
	hnswHeaderSize = 64
)

// Metric byte encoding
const (
	metricByteL2     uint8 = 0
	metricByteCosine uint8 = 1
	metricByteIP     uint8 = 2
)

// HNSWWrapper provides an HNSW index implementation.
// This is a pure Go implementation without external dependencies.
type HNSWWrapper struct {
	nodes      map[uint64]*hnswNode
	entryPoint uint64
	hasEntry   bool

	dimensions uint32
	metric     types.DistanceMetric
	filePath   string

	// HNSW parameters
	M              int     // Max number of connections per layer
	Ml             float64 // Level normalization factor
	EfConstruction int     // Size of dynamic candidate list during construction
	EfSearch       int     // Size of dynamic candidate list during search
	MaxLevel       int     // Maximum level in the graph

	dirty bool // Set on Add/Delete, cleared on Save
	mu    sync.RWMutex
}

// hnswNode represents a node in the HNSW graph.
type hnswNode struct {
	ID        uint64
	Vector    []float32
	Level     int
	Neighbors [][]uint64 // neighbors[level] = list of neighbor IDs
}

// NewHNSWWrapper creates a new HNSW wrapper with the given configuration.
func NewHNSWWrapper(dims uint32, metric types.DistanceMetric, filePath string) (*HNSWWrapper, error) {
	return &HNSWWrapper{
		nodes:          make(map[uint64]*hnswNode),
		dimensions:     dims,
		metric:         metric,
		filePath:       filePath,
		M:              16,
		Ml:             1.0 / math.Log(16),
		EfConstruction: 200,
		EfSearch:       100,
		MaxLevel:       0,
	}, nil
}

// distanceL2 calculates squared Euclidean distance.
func distanceL2(a, b []float32) float32 {
	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return sum
}

// distanceCosine calculates cosine distance (1 - cosine similarity).
func distanceCosine(a, b []float32) float32 {
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 1.0
	}
	return 1.0 - (dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB)))))
}

// distanceIP calculates negative inner product (for max inner product search).
func distanceIP(a, b []float32) float32 {
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	return -dot // Negative because we want to maximize IP
}

// distance calculates distance between two vectors using the configured metric.
func (hw *HNSWWrapper) distance(a, b []float32) float32 {
	switch hw.metric {
	case types.MetricCosine:
		return distanceCosine(a, b)
	case types.MetricIP:
		return distanceIP(a, b)
	case types.MetricL2:
		fallthrough
	default:
		return distanceL2(a, b)
	}
}

// randomLevel generates a random level for a new node.
func (hw *HNSWWrapper) randomLevel() int {
	level := 0
	for rand.Float64() < hw.Ml && level < 32 {
		level++
	}
	return level
}

// Add inserts a vector with the given ID.
func (hw *HNSWWrapper) Add(vectorID uint64, vector []float32) error {
	hw.mu.Lock()
	defer hw.mu.Unlock()

	if uint32(len(vector)) != hw.dimensions {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", hw.dimensions, len(vector))
	}

	if _, exists := hw.nodes[vectorID]; exists {
		return fmt.Errorf("vector ID %d already exists", vectorID)
	}

	level := hw.randomLevel()
	node := &hnswNode{
		ID:        vectorID,
		Vector:    make([]float32, len(vector)),
		Level:     level,
		Neighbors: make([][]uint64, level+1),
	}
	copy(node.Vector, vector)
	for i := range node.Neighbors {
		node.Neighbors[i] = make([]uint64, 0, hw.M)
	}

	if !hw.hasEntry {
		hw.nodes[vectorID] = node
		hw.entryPoint = vectorID
		hw.hasEntry = true
		hw.MaxLevel = level
		hw.dirty = true
		return nil
	}

	// Find entry point at the top level
	ep := hw.entryPoint
	for l := hw.MaxLevel; l > level; l-- {
		ep = hw.searchLayer(vector, ep, 1, l)[0].ID
	}

	// Insert at each level
	for l := min(level, hw.MaxLevel); l >= 0; l-- {
		neighbors := hw.searchLayer(vector, ep, hw.EfConstruction, l)
		selectedNeighbors := hw.selectNeighbors(vector, neighbors, hw.M, l)

		node.Neighbors[l] = make([]uint64, 0, len(selectedNeighbors))
		for _, n := range selectedNeighbors {
			node.Neighbors[l] = append(node.Neighbors[l], n.ID)
			// Add reverse connection
			hw.addConnection(n.ID, vectorID, l)
		}

		if len(neighbors) > 0 {
			ep = neighbors[0].ID
		}
	}

	hw.nodes[vectorID] = node
	hw.dirty = true

	if level > hw.MaxLevel {
		hw.MaxLevel = level
		hw.entryPoint = vectorID
	}

	return nil
}

// candidate represents a search candidate.
type candidate struct {
	ID       uint64
	Distance float32
}

// candidateHeap is a min-heap of candidates.
type candidateHeap []candidate

func (h candidateHeap) Len() int           { return len(h) }
func (h candidateHeap) Less(i, j int) bool { return h[i].Distance < h[j].Distance }
func (h candidateHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *candidateHeap) Push(x any)        { *h = append(*h, x.(candidate)) }
func (h *candidateHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// maxCandidateHeap is a max-heap of candidates.
type maxCandidateHeap []candidate

func (h maxCandidateHeap) Len() int           { return len(h) }
func (h maxCandidateHeap) Less(i, j int) bool { return h[i].Distance > h[j].Distance }
func (h maxCandidateHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *maxCandidateHeap) Push(x any)        { *h = append(*h, x.(candidate)) }
func (h *maxCandidateHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// searchLayer performs a greedy search at a given layer.
func (hw *HNSWWrapper) searchLayer(query []float32, entryID uint64, ef int, level int) []candidate {
	visited := make(map[uint64]bool)

	entryNode := hw.nodes[entryID]
	if entryNode == nil {
		return nil
	}

	entryDist := hw.distance(query, entryNode.Vector)

	candidates := &candidateHeap{{ID: entryID, Distance: entryDist}}
	heap.Init(candidates)

	results := &maxCandidateHeap{{ID: entryID, Distance: entryDist}}
	heap.Init(results)

	visited[entryID] = true

	for candidates.Len() > 0 {
		current := heap.Pop(candidates).(candidate)

		if results.Len() > 0 && current.Distance > (*results)[0].Distance && results.Len() >= ef {
			break
		}

		node := hw.nodes[current.ID]
		if node == nil || level >= len(node.Neighbors) {
			continue
		}

		for _, neighborID := range node.Neighbors[level] {
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			neighborNode := hw.nodes[neighborID]
			if neighborNode == nil {
				continue
			}

			dist := hw.distance(query, neighborNode.Vector)

			if results.Len() < ef || dist < (*results)[0].Distance {
				heap.Push(candidates, candidate{ID: neighborID, Distance: dist})
				heap.Push(results, candidate{ID: neighborID, Distance: dist})

				if results.Len() > ef {
					heap.Pop(results)
				}
			}
		}
	}

	// Convert results to slice sorted by distance
	resultSlice := make([]candidate, results.Len())
	for i := len(resultSlice) - 1; i >= 0; i-- {
		resultSlice[i] = heap.Pop(results).(candidate)
	}
	return resultSlice
}

// selectNeighbors selects the best neighbors from candidates.
func (hw *HNSWWrapper) selectNeighbors(query []float32, candidates []candidate, m int, level int) []candidate {
	if len(candidates) <= m {
		return candidates
	}
	return candidates[:m]
}

// addConnection adds a connection from source to target at the given level.
func (hw *HNSWWrapper) addConnection(sourceID, targetID uint64, level int) {
	source := hw.nodes[sourceID]
	if source == nil || level >= len(source.Neighbors) {
		return
	}

	// Check if connection already exists
	for _, n := range source.Neighbors[level] {
		if n == targetID {
			return
		}
	}

	source.Neighbors[level] = append(source.Neighbors[level], targetID)

	// Prune if too many connections
	if len(source.Neighbors[level]) > hw.M*2 {
		hw.pruneConnections(sourceID, level)
	}
}

// pruneConnections removes excess connections for a node at a level.
func (hw *HNSWWrapper) pruneConnections(nodeID uint64, level int) {
	node := hw.nodes[nodeID]
	if node == nil || level >= len(node.Neighbors) {
		return
	}

	// Calculate distances to all neighbors
	candidates := make([]candidate, 0, len(node.Neighbors[level]))
	for _, neighborID := range node.Neighbors[level] {
		neighbor := hw.nodes[neighborID]
		if neighbor != nil {
			dist := hw.distance(node.Vector, neighbor.Vector)
			candidates = append(candidates, candidate{ID: neighborID, Distance: dist})
		}
	}

	// Sort by distance and keep only M
	selected := hw.selectNeighbors(node.Vector, candidates, hw.M, level)
	node.Neighbors[level] = make([]uint64, 0, len(selected))
	for _, c := range selected {
		node.Neighbors[level] = append(node.Neighbors[level], c.ID)
	}
}

// HNSWSearchResult represents a single search result from HNSW.
type HNSWSearchResult struct {
	VectorID uint64
	Distance float32
}

// Search performs ANN search and returns the k nearest neighbors.
func (hw *HNSWWrapper) Search(query []float32, k int, filter *BitSet) ([]HNSWSearchResult, error) {
	hw.mu.RLock()
	defer hw.mu.RUnlock()

	if uint32(len(query)) != hw.dimensions {
		return nil, fmt.Errorf("query dimension mismatch: expected %d, got %d", hw.dimensions, len(query))
	}

	if !hw.hasEntry {
		return nil, nil
	}

	// If we have a filter, search for more results
	searchK := k
	hasFilter := filter != nil && !filter.IsEmpty()
	if hasFilter {
		searchK = k * 10
		if searchK > len(hw.nodes) {
			searchK = len(hw.nodes)
		}
	}

	// Navigate from top level to level 0
	ep := hw.entryPoint
	for l := hw.MaxLevel; l > 0; l-- {
		candidates := hw.searchLayer(query, ep, 1, l)
		if len(candidates) > 0 {
			ep = candidates[0].ID
		}
	}

	// Search at level 0
	candidates := hw.searchLayer(query, ep, max(searchK, hw.EfSearch), 0)

	results := make([]HNSWSearchResult, 0, k)
	for _, c := range candidates {
		if hasFilter && !filter.Contains(c.ID) {
			continue
		}
		results = append(results, HNSWSearchResult{
			VectorID: c.ID,
			Distance: c.Distance,
		})
		if len(results) >= k {
			break
		}
	}

	return results, nil
}

// Delete marks a vector for deletion.
func (hw *HNSWWrapper) Delete(vectorID uint64) error {
	hw.mu.Lock()
	defer hw.mu.Unlock()

	node := hw.nodes[vectorID]
	if node == nil {
		return fmt.Errorf("vector ID %d not found", vectorID)
	}

	// Remove connections from neighbors
	for level, neighbors := range node.Neighbors {
		for _, neighborID := range neighbors {
			hw.removeConnection(neighborID, vectorID, level)
		}
	}

	// Remove the node
	delete(hw.nodes, vectorID)
	hw.dirty = true

	// Update entry point if needed
	if hw.entryPoint == vectorID {
		hw.updateEntryPoint()
	}

	return nil
}

// removeConnection removes a connection from source to target.
func (hw *HNSWWrapper) removeConnection(sourceID, targetID uint64, level int) {
	source := hw.nodes[sourceID]
	if source == nil || level >= len(source.Neighbors) {
		return
	}

	newNeighbors := make([]uint64, 0, len(source.Neighbors[level]))
	for _, n := range source.Neighbors[level] {
		if n != targetID {
			newNeighbors = append(newNeighbors, n)
		}
	}
	source.Neighbors[level] = newNeighbors
}

// updateEntryPoint finds a new entry point after deletion.
func (hw *HNSWWrapper) updateEntryPoint() {
	hw.hasEntry = false
	hw.MaxLevel = 0

	for id, node := range hw.nodes {
		if !hw.hasEntry || node.Level > hw.MaxLevel {
			hw.entryPoint = id
			hw.MaxLevel = node.Level
			hw.hasEntry = true
		}
	}
}

// metricToByte converts distance metric to byte encoding.
func metricToByte(m types.DistanceMetric) uint8 {
	switch m {
	case types.MetricCosine:
		return metricByteCosine
	case types.MetricIP:
		return metricByteIP
	default:
		return metricByteL2
	}
}

// byteToMetric converts byte encoding to distance metric.
func byteToMetric(b uint8) types.DistanceMetric {
	switch b {
	case metricByteCosine:
		return types.MetricCosine
	case metricByteIP:
		return types.MetricIP
	default:
		return types.MetricL2
	}
}

// Save persists the HNSW index to disk in binary format.
func (hw *HNSWWrapper) Save() error {
	hw.mu.RLock()
	defer hw.mu.RUnlock()

	file, err := os.Create(hw.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Collect and sort node IDs for deterministic output
	nodeIDs := make([]uint64, 0, len(hw.nodes))
	for id := range hw.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Slice(nodeIDs, func(i, j int) bool { return nodeIDs[i] < nodeIDs[j] })

	// Calculate offsets
	vectorSize := hw.dimensions * 4 // float32 = 4 bytes
	nodeTableSize := uint32(len(hw.nodes)) * 24
	vectorSectionOffset := uint32(hnswHeaderSize) + nodeTableSize

	// Build node entries and calculate neighbor offsets
	type nodeEntry struct {
		id             uint64
		level          int32
		vectorOffset   uint32
		neighborOffset uint32
		neighborCount  uint32
	}
	entries := make([]nodeEntry, len(nodeIDs))
	neighborOffset := uint32(0)

	for i, id := range nodeIDs {
		node := hw.nodes[id]
		// Count total neighbors across all levels
		totalNeighbors := uint32(0)
		for _, neighbors := range node.Neighbors {
			totalNeighbors += uint32(len(neighbors))
		}
		// Neighbor section: 2 bytes levelCount + (2 bytes count + N*8 bytes neighbors) per level
		neighborSize := uint32(2) // levelCount
		for _, neighbors := range node.Neighbors {
			neighborSize += 2 + uint32(len(neighbors))*8
		}

		entries[i] = nodeEntry{
			id:             id,
			level:          int32(node.Level),
			vectorOffset:   uint32(i) * vectorSize,
			neighborOffset: neighborOffset,
			neighborCount:  totalNeighbors,
		}
		neighborOffset += neighborSize
	}

	neighborSectionOffset := vectorSectionOffset + uint32(len(hw.nodes))*vectorSize

	// Write header (64 bytes)
	header := make([]byte, hnswHeaderSize)
	copy(header[0:8], hnswMagic)
	binary.LittleEndian.PutUint32(header[8:12], hw.dimensions)
	header[12] = metricToByte(hw.metric)
	// header[13:16] reserved
	binary.LittleEndian.PutUint32(header[16:20], uint32(len(hw.nodes)))
	binary.LittleEndian.PutUint64(header[20:28], hw.entryPoint)
	binary.LittleEndian.PutUint32(header[28:32], uint32(hw.MaxLevel))
	binary.LittleEndian.PutUint32(header[32:36], uint32(hw.M))
	if hw.hasEntry {
		header[36] = 1
	}
	// header[37:64] reserved

	if _, err := file.Write(header); err != nil {
		return err
	}

	// Write node table
	for _, entry := range entries {
		nodeBuf := make([]byte, 24)
		binary.LittleEndian.PutUint64(nodeBuf[0:8], entry.id)
		binary.LittleEndian.PutUint32(nodeBuf[8:12], uint32(entry.level))
		binary.LittleEndian.PutUint32(nodeBuf[12:16], entry.vectorOffset)
		binary.LittleEndian.PutUint32(nodeBuf[16:20], neighborSectionOffset+entry.neighborOffset)
		binary.LittleEndian.PutUint32(nodeBuf[20:24], entry.neighborCount)
		if _, err := file.Write(nodeBuf); err != nil {
			return err
		}
	}

	// Write vector data
	for _, id := range nodeIDs {
		node := hw.nodes[id]
		for _, v := range node.Vector {
			if err := binary.Write(file, binary.LittleEndian, v); err != nil {
				return err
			}
		}
	}

	// Write neighbor lists
	for _, id := range nodeIDs {
		node := hw.nodes[id]
		// Write level count
		if err := binary.Write(file, binary.LittleEndian, uint16(len(node.Neighbors))); err != nil {
			return err
		}
		for _, neighbors := range node.Neighbors {
			// Write neighbor count for this level
			if err := binary.Write(file, binary.LittleEndian, uint16(len(neighbors))); err != nil {
				return err
			}
			// Write neighbor IDs
			for _, nid := range neighbors {
				if err := binary.Write(file, binary.LittleEndian, nid); err != nil {
					return err
				}
			}
		}
	}

	hw.dirty = false
	return nil
}

// Load reads an HNSW index from disk in binary format.
func (hw *HNSWWrapper) Load() error {
	hw.mu.Lock()
	defer hw.mu.Unlock()

	if _, err := os.Stat(hw.filePath); os.IsNotExist(err) {
		return nil
	}

	file, err := os.Open(hw.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read header
	header := make([]byte, hnswHeaderSize)
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	// Validate magic
	if string(header[0:8]) != hnswMagic {
		return errors.New("invalid HNSW file: wrong magic number")
	}

	// Parse header
	dimensions := binary.LittleEndian.Uint32(header[8:12])
	metric := byteToMetric(header[12])
	nodeCount := binary.LittleEndian.Uint32(header[16:20])
	entryPoint := binary.LittleEndian.Uint64(header[20:28])
	maxLevel := int(binary.LittleEndian.Uint32(header[28:32]))
	// M at header[32:36] - we use our configured value
	hasEntry := header[36] == 1

	// Validate
	if dimensions != hw.dimensions {
		return fmt.Errorf("dimension mismatch: file has %d, expected %d", dimensions, hw.dimensions)
	}
	if metric != hw.metric {
		return fmt.Errorf("metric mismatch: file has %s, expected %s", metric, hw.metric)
	}

	// Read node table
	type nodeEntry struct {
		id             uint64
		level          int32
		vectorOffset   uint32
		neighborOffset uint32
		neighborCount  uint32
	}
	entries := make([]nodeEntry, nodeCount)
	for i := uint32(0); i < nodeCount; i++ {
		nodeBuf := make([]byte, 24)
		if _, err := io.ReadFull(file, nodeBuf); err != nil {
			return fmt.Errorf("failed to read node table entry %d: %w", i, err)
		}
		entries[i] = nodeEntry{
			id:             binary.LittleEndian.Uint64(nodeBuf[0:8]),
			level:          int32(binary.LittleEndian.Uint32(nodeBuf[8:12])),
			vectorOffset:   binary.LittleEndian.Uint32(nodeBuf[12:16]),
			neighborOffset: binary.LittleEndian.Uint32(nodeBuf[16:20]),
			neighborCount:  binary.LittleEndian.Uint32(nodeBuf[20:24]),
		}
	}

	// Read vectors
	nodes := make(map[uint64]*hnswNode)
	for _, entry := range entries {
		vector := make([]float32, dimensions)
		for j := uint32(0); j < dimensions; j++ {
			if err := binary.Read(file, binary.LittleEndian, &vector[j]); err != nil {
				return fmt.Errorf("failed to read vector for node %d: %w", entry.id, err)
			}
		}
		nodes[entry.id] = &hnswNode{
			ID:     entry.id,
			Vector: vector,
			Level:  int(entry.level),
		}
	}

	// Read neighbor lists
	for _, entry := range entries {
		node := nodes[entry.id]
		var levelCount uint16
		if err := binary.Read(file, binary.LittleEndian, &levelCount); err != nil {
			return fmt.Errorf("failed to read level count for node %d: %w", entry.id, err)
		}
		node.Neighbors = make([][]uint64, levelCount)
		for l := uint16(0); l < levelCount; l++ {
			var neighborCount uint16
			if err := binary.Read(file, binary.LittleEndian, &neighborCount); err != nil {
				return fmt.Errorf("failed to read neighbor count for node %d level %d: %w", entry.id, l, err)
			}
			node.Neighbors[l] = make([]uint64, neighborCount)
			for n := uint16(0); n < neighborCount; n++ {
				if err := binary.Read(file, binary.LittleEndian, &node.Neighbors[l][n]); err != nil {
					return fmt.Errorf("failed to read neighbor for node %d: %w", entry.id, err)
				}
			}
		}
	}

	hw.nodes = nodes
	hw.entryPoint = entryPoint
	hw.hasEntry = hasEntry
	hw.MaxLevel = maxLevel
	hw.dirty = false

	return nil
}

// IsDirty returns true if the index has unsaved changes.
func (hw *HNSWWrapper) IsDirty() bool {
	hw.mu.RLock()
	defer hw.mu.RUnlock()
	return hw.dirty
}

// Count returns the number of vectors in the index.
func (hw *HNSWWrapper) Count() uint64 {
	hw.mu.RLock()
	defer hw.mu.RUnlock()
	return uint64(len(hw.nodes))
}

// Dimensions returns the configured dimensions.
func (hw *HNSWWrapper) Dimensions() uint32 {
	return hw.dimensions
}

// Metric returns the configured distance metric.
func (hw *HNSWWrapper) Metric() types.DistanceMetric {
	return hw.metric
}

// Close releases all resources held by the index.
func (hw *HNSWWrapper) Close() error {
	return nil
}

// Contains checks if a vector ID exists in the index.
func (hw *HNSWWrapper) Contains(vectorID uint64) bool {
	hw.mu.RLock()
	defer hw.mu.RUnlock()
	_, exists := hw.nodes[vectorID]
	return exists
}

// CollectionMeta holds collection metadata for persistence.
type CollectionMeta struct {
	Name       string               `json:"name"`
	Dimensions uint32               `json:"dimensions"`
	Metric     types.DistanceMetric `json:"metric"`
}

// ValidateCollectionConfig validates collection configuration.
func ValidateCollectionConfig(config *types.CollectionConfig) error {
	if config.Name == "" {
		return errors.New("collection name cannot be empty")
	}
	if config.Dimensions == 0 {
		return errors.New("dimensions must be greater than 0")
	}
	switch config.Metric {
	case types.MetricL2, types.MetricCosine, types.MetricIP:
		// Valid
	default:
		return fmt.Errorf("invalid metric: %s", config.Metric)
	}
	return nil
}

// SaveCollectionMeta saves collection metadata to meta.json.
func SaveCollectionMeta(basePath string, meta *CollectionMeta) error {
	metaPath := filepath.Join(basePath, "meta.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0644)
}

// LoadCollectionMeta loads collection metadata from meta.json.
func LoadCollectionMeta(basePath string) (*CollectionMeta, error) {
	metaPath := filepath.Join(basePath, "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var meta CollectionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
