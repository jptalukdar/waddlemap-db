package types

// DistanceMetric represents the distance metric used for vector similarity.
type DistanceMetric string

const (
	MetricL2     DistanceMetric = "l2"     // Euclidean distance
	MetricCosine DistanceMetric = "cosine" // Cosine similarity
	MetricIP     DistanceMetric = "ip"     // Inner product
)

// DataType identifies the type of data stored in an entry.
type DataType uint8

const (
	DataTypeBinary DataType = 0b000 // Standard binary data
	DataTypeVector DataType = 0b001 // Vector data (binary + vector)
)

// EntryFlags represents the flags byte in the entry header.
type EntryFlags struct {
	DataType   DataType // Bits 0-2: data type
	Compressed bool     // Bit 3: compression flag
	Tombstone  bool     // Bit 4: deleted entry flag
}

// CollectionConfig holds metadata for a vector collection.
type CollectionConfig struct {
	Name       string         `json:"name"`       // Unique collection name
	Dimensions uint32         `json:"dimensions"` // Fixed vector dimensions
	Metric     DistanceMetric `json:"metric"`     // Distance metric: "l2" | "cosine" | "ip"
}

// KeywordEntry represents keyword metadata for a vector entry.
type KeywordEntry struct {
	Keywords []string // Normalized, lowercase tokens
	VectorID uint64   // Reference to HNSW vector
	KeyName  string   // Associated key name
}

// SearchFilter defines filters for vector/keyword searches.
type SearchFilter struct {
	Keys        []string // Limit to specific keys (empty = all)
	Keywords    []string // Keyword filter
	KeywordMode string   // "exact"|"prefix"|"partial"|"levenshtein"
	MaxDistance uint32   // For levenshtein mode
}

// VectorSearchResult holds a single result from a vector search.
type VectorSearchResult struct {
	Key      string  // The key of the matching entry
	Distance float32 // Distance from the query vector
	VectorID uint64  // Internal vector ID
	Data     []byte  // Optional payload data
}

// BlockData represents a single block of data.
type BlockData struct {
	Primary  string    // Primary text/binary data
	Vector   []float32 // Secondary vector data
	Keywords []string  // Keywords
}

// SearchResultItem holds a result from block-based search.
type SearchResultItem struct {
	Key      string     // Document Key
	Index    uint32     // Block Index
	Distance float32    // Distance
	Block    *BlockData // Optional block content
}

// ParseFlags converts a flags byte to EntryFlags struct.
func ParseFlags(flags uint8) EntryFlags {
	return EntryFlags{
		DataType:   DataType(flags & 0b00000111),
		Compressed: (flags & 0b00001000) != 0,
		Tombstone:  (flags & 0b00010000) != 0,
	}
}

// EncodeFlags converts an EntryFlags struct to a flags byte.
func EncodeFlags(ef EntryFlags) uint8 {
	var flags uint8 = uint8(ef.DataType) & 0b00000111
	if ef.Compressed {
		flags |= 0b00001000
	}
	if ef.Tombstone {
		flags |= 0b00010000
	}
	return flags
}
