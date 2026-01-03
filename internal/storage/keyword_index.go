package storage

import (
	"encoding/gob"
	"os"
	"strings"
	"sync"
)

// InvertedIndex stores trigram → postings list mappings for keyword search.
// This corresponds to the keywords.inv file in the spec.
type InvertedIndex struct {
	// index maps trigrams to lists of VectorIDs
	index    map[string][]uint64
	filePath string
	mu       sync.RWMutex
}

// NewInvertedIndex creates a new inverted index.
func NewInvertedIndex(filePath string) *InvertedIndex {
	return &InvertedIndex{
		index:    make(map[string][]uint64),
		filePath: filePath,
	}
}

// GenerateTrigrams generates trigrams from a keyword.
// Example: "finance" → ["fin", "ina", "nan", "anc", "nce"]
func GenerateTrigrams(keyword string) []string {
	keyword = strings.ToLower(keyword)
	if len(keyword) < 3 {
		// For short keywords, use the keyword itself as a "trigram"
		return []string{keyword}
	}

	trigrams := make([]string, 0, len(keyword)-2)
	runes := []rune(keyword)
	for i := 0; i <= len(runes)-3; i++ {
		trigrams = append(trigrams, string(runes[i:i+3]))
	}
	return trigrams
}

// Add indexes keywords for a given VectorID.
func (ii *InvertedIndex) Add(keywords []string, vectorID uint64) {
	ii.mu.Lock()
	defer ii.mu.Unlock()

	for _, kw := range keywords {
		kw = strings.ToLower(kw)
		trigrams := GenerateTrigrams(kw)
		for _, tg := range trigrams {
			ii.index[tg] = appendUnique(ii.index[tg], vectorID)
		}
		// Also index the full keyword for exact match
		ii.index["kw:"+kw] = appendUnique(ii.index["kw:"+kw], vectorID)
	}
}

// Delete removes keyword indexing for a given VectorID.
func (ii *InvertedIndex) Delete(keywords []string, vectorID uint64) {
	ii.mu.Lock()
	defer ii.mu.Unlock()

	for _, kw := range keywords {
		kw = strings.ToLower(kw)
		trigrams := GenerateTrigrams(kw)
		for _, tg := range trigrams {
			ii.index[tg] = removeValue(ii.index[tg], vectorID)
		}
		ii.index["kw:"+kw] = removeValue(ii.index["kw:"+kw], vectorID)
	}
}

// SearchExact finds VectorIDs that have all the specified keywords (exact match).
func (ii *InvertedIndex) SearchExact(keywords []string) *BitSet {
	ii.mu.RLock()
	defer ii.mu.RUnlock()

	if len(keywords) == 0 {
		return nil
	}

	// Start with first keyword's matches
	kw := strings.ToLower(keywords[0])
	result := NewBitSetFromSlice(ii.index["kw:"+kw])

	// Intersect with remaining keywords
	for _, kw := range keywords[1:] {
		kw = strings.ToLower(kw)
		other := NewBitSetFromSlice(ii.index["kw:"+kw])
		result = result.Intersect(other)
	}

	return result
}

// SearchPrefix finds VectorIDs that have keywords starting with the given prefixes.
func (ii *InvertedIndex) SearchPrefix(prefixes []string) *BitSet {
	ii.mu.RLock()
	defer ii.mu.RUnlock()

	if len(prefixes) == 0 {
		return nil
	}

	var result *BitSet
	for _, prefix := range prefixes {
		prefix = strings.ToLower(prefix)
		candidates := NewBitSet()

		// Find all keywords starting with this prefix
		for key, ids := range ii.index {
			if strings.HasPrefix(key, "kw:") {
				keyword := strings.TrimPrefix(key, "kw:")
				if strings.HasPrefix(keyword, prefix) {
					for _, id := range ids {
						candidates.Set(id)
					}
				}
			}
		}

		if result == nil {
			result = candidates
		} else {
			result = result.Intersect(candidates)
		}
	}

	return result
}

// SearchPartial finds VectorIDs that have keywords containing the given substrings.
func (ii *InvertedIndex) SearchPartial(substrings []string) *BitSet {
	ii.mu.RLock()
	defer ii.mu.RUnlock()

	if len(substrings) == 0 {
		return nil
	}

	var result *BitSet
	for _, substr := range substrings {
		substr = strings.ToLower(substr)
		candidates := NewBitSet()

		// Use trigrams to find candidates
		trigrams := GenerateTrigrams(substr)
		if len(trigrams) > 0 {
			// Start with first trigram's matches
			for _, id := range ii.index[trigrams[0]] {
				candidates.Set(id)
			}
			// Intersect with remaining trigrams
			for _, tg := range trigrams[1:] {
				other := NewBitSetFromSlice(ii.index[tg])
				candidates = candidates.Intersect(other)
			}
		}

		if result == nil {
			result = candidates
		} else {
			result = result.Intersect(candidates)
		}
	}

	return result
}

// SearchLevenshtein finds VectorIDs with keywords within Levenshtein distance.
func (ii *InvertedIndex) SearchLevenshtein(keywords []string, maxDistance uint32) *BitSet {
	ii.mu.RLock()
	defer ii.mu.RUnlock()

	if len(keywords) == 0 {
		return nil
	}

	var result *BitSet
	for _, query := range keywords {
		query = strings.ToLower(query)
		candidates := NewBitSet()

		// Scan all indexed keywords
		for key, ids := range ii.index {
			if strings.HasPrefix(key, "kw:") {
				keyword := strings.TrimPrefix(key, "kw:")
				if levenshteinDistance(query, keyword) <= int(maxDistance) {
					for _, id := range ids {
						candidates.Set(id)
					}
				}
			}
		}

		if result == nil {
			result = candidates
		} else {
			result = result.Intersect(candidates)
		}
	}

	return result
}

// Search performs a keyword search with the specified mode.
func (ii *InvertedIndex) Search(keywords []string, mode string, maxDistance uint32) *BitSet {
	switch mode {
	case "exact":
		return ii.SearchExact(keywords)
	case "prefix":
		return ii.SearchPrefix(keywords)
	case "partial":
		return ii.SearchPartial(keywords)
	case "levenshtein":
		return ii.SearchLevenshtein(keywords, maxDistance)
	default:
		return ii.SearchExact(keywords)
	}
}

// Save persists the inverted index to disk.
func (ii *InvertedIndex) Save() error {
	ii.mu.RLock()
	defer ii.mu.RUnlock()

	file, err := os.Create(ii.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(ii.index)
}

// Load reads the inverted index from disk.
func (ii *InvertedIndex) Load() error {
	ii.mu.Lock()
	defer ii.mu.Unlock()

	file, err := os.Open(ii.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			ii.index = make(map[string][]uint64)
			return nil
		}
		return err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	return decoder.Decode(&ii.index)
}

// Helper functions

func appendUnique(slice []uint64, value uint64) []uint64 {
	for _, v := range slice {
		if v == value {
			return slice
		}
	}
	return append(slice, value)
}

func removeValue(slice []uint64, value uint64) []uint64 {
	result := make([]uint64, 0, len(slice))
	for _, v := range slice {
		if v != value {
			result = append(result, v)
		}
	}
	return result
}

// levenshteinDistance calculates the Levenshtein distance between two strings.
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	ra := []rune(a)
	rb := []rune(b)

	// Create distance matrix
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
	}

	// Initialize first column
	for i := 0; i <= len(ra); i++ {
		d[i][0] = i
	}
	// Initialize first row
	for j := 0; j <= len(rb); j++ {
		d[0][j] = j
	}

	// Fill in the rest
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(
				d[i-1][j]+1,      // deletion
				d[i][j-1]+1,      // insertion
				d[i-1][j-1]+cost, // substitution
			)
		}
	}

	return d[len(ra)][len(rb)]
}
