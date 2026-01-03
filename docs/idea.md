## Vector Store Mode Plan

### Overview

- Add a vector store mode to support localized data and keyword-based retrieval.
- Enable storage of both traditional (binary) and vector data within the same key-value database.

### Design

#### 1. Data Storage

- **HNSW (Hierarchical Navigable Small World):**  
	Use HNSW for storing vector data, enabling efficient Approximate Nearest Neighbor (ANN) search.
- **Key-Value DB as Index/Collection:**  
	- Each key represents a collection name.
	- Each value is a pointer/link to the corresponding HNSW store.

#### 2. Backward Compatibility

- The key-value DB must continue to support traditional binary data.
- Users can store binary and vector data side by side.

#### 3. Metadata & Data Typing

- Add metadata to each key to identify the data type:
	- Reserve 3 bits for data type flags.
		- `000`: Standard binary data
        - `001`: vector data
		- Other combinations: Reserved for future/special types
- **Data Structure:**
	```
	[key] - [data type (3 bits)] [metadata] [actual data] [address/location of external data]
	```
	- External data may refer to the HNSW datastore.
	- Metadata may include a list of keyword tokens.

#### 4. Keyword Indexing

- Allow users to index specific tokens for faster retrieval.
- Metadata should support a list of keyword tokens.

#### 5. Collections

- Support collections consisting of multiple keys.
- Data cannot be moved between collections, only copied.
- All vector values in a collection point to the same HNSW datastore.

#### 6. ANN Support

- Integrate Approximate Nearest Neighbor (ANN) search capabilities via HNSW.

---

## Finalized Design Decisions

### Design Resolutions

| Aspect | Decision |
|--------|----------|
| **HNSW Library** | Any battle-tested pure Go or CGo library (no external vector DB) |
| **Vector Dimensions** | Fixed per collection, varies between collections |
| **Distance Metrics** | Support all three: L2 (Euclidean), Cosine, Inner Product |
| **Collection vs Key** | Separate entities; same name allowed for both |
| **Mixed Data Types** | Keys can store both binary AND vector data simultaneously |
| **HNSW Storage** | Separate file per collection with HNSW-based sharding |
| **Backward Compatibility** | Not required (fresh implementation) |

---

### Keyword Specification

| Property | Rule |
|----------|------|
| Case Sensitivity | Case-insensitive (stored as lowercase) |
| Allowed Characters | `a-z`, `0-9`, `_` (underscore), `-` (dash) |
| Max Length | 128 bytes per keyword |
| Count Limit | Unlimited keywords per entry |

**Search Modes** (user-selectable):
1. `exact` - Exact string match
2. `prefix` - Prefix matching
3. `partial` - Substring matching
4. `levenshtein` - Fuzzy matching with configurable distance

---

### Core Concept: Data Model

**Hierarchy:**
```
Collection
  └── Keys
        ├── Keywords []string
        ├── Primary Data (binary)
        └── Secondary Data (vector → HNSW)
```

**Example:**
```
collection: "documents"
  └── key: "doc_123"
        ├── keywords: ["finance", "q4-report"]
        ├── primary: "Full document text..."
        └── secondary: [0.1, 0.2, ...] → HNSW index
```

**Search Flow:**
1. User provides query vector + optional filters (keys, keywords)
2. If filters given → limit search to matching keys only
3. HNSW search within filtered set
4. Return matching keys with distances

---

### Data Structure Updates

#### Entry Encoding
```
[key] - [data_type (3 bits)] [metadata] [binary_data] [secondary data/pointer]
```

**Data Type Flags** (secondary data indicator):
- `000` - No secondary data (binary only)
- `001` - Vector secondary data (binary + vector)
- `010-111` - Reserved for future secondary data types

#### Collection Metadata
```go
CollectionConfig {
    Name       string         // Unique collection name
    Dimensions uint32         // Fixed vector dimensions
    Metric     DistanceMetric // "l2" | "cosine" | "ip"
}
```

#### Keyword Entry
```go
KeywordEntry {
    Keywords []string  // Normalized, lowercase tokens
    VectorID uint64    // Reference to HNSW vector
    KeyName  string    // Associated key name
}
```

---

### Storage Format

**KV Record Format:**
```
[Key][Headers][Primary Data][Secondary Data / Index Pointer]
```

Headers define lengths for variable-size fields.

---

### Directory Structure

```
waddlemap_db/
├── data/                       # Source of Truth (KV Store)
│   ├── shard_001.db            # Key → [Header][BinaryBlob][VectorID]
│   └── ...
└── indexes/                    # Derived Data (Rebuildable)
    └── {collection_name}/
        ├── vectors.hnsw        # HNSW Graph (mmap-backed)
        ├── keywords.inv        # Inverted Index (Trigram postings)
        ├── doc_map.bin         # Forward Index (VectorID → Key)
        └── meta.json           # Config (dims, metric, immutable)
```

**Key Principle:** Indexes are derived and rebuildable from data.

---

### HNSW Memory Management

> **Risk:** 5M vectors × 1536 dims = ~30GB+ RAM → OOM crash

**Solution:** Use mmap-backed HNSW library
- Graph file is memory-mapped, not fully loaded
- OS manages page caching
- Enables handling collections larger than RAM

---

### Keyword Indexing

**Method:** Trigram (3-gram) Indexing

Example: `"finance"` → `["fin", "ina", "nan", "anc", "nce"]`

**Benefits:**
- Partial match via set intersection (not full scan)
- Prefix/suffix matching
- Levenshtein approximation via trigram overlap

**Storage:** Inverted index with postings lists
```
trigram → [key1, key2, key3, ...]
```

---

### Filtered Search Algorithm

**Method:** Iterative Filtered Search (Post-filtering during traversal)

```
1. Keyword Lookup:
   - If keywords provided → query Inverted Index
   - Build BitSet of valid key IDs

2. HNSW Traversal:
   - Enter graph at entry point
   - For each candidate node:
     a. Calculate distance
     b. Check: Is node ID in KeywordBitSet?
        - YES → Add to results heap
        - NO  → Discard from results
     c. ALWAYS add to traversal queue (maintain connectivity)

3. Return top-K from results heap
```

**Key:** Non-matching nodes are visited but not returned.

---

### Consistency & Durability

**Challenge:** Two files must stay in sync (KV data + HNSW index)

**Options:**
1. **WAL (Write-Ahead Log):** Single WAL covers both writes atomically
2. **Repair-on-Read:** Detect missing links, clean up orphans on load

**Recommendation:** WAL for writes + repair-on-read for crash recovery

---

### Immutability Rules

| Property | Mutable? |
|----------|----------|
| Collection Name | No |
| Dimensions | No (fixed at creation) |
| Distance Metric | No (fixed at creation) |
| Keys/Vectors | Yes (add/update/delete) |

---

### API Operations

#### Collection Management
- `CreateCollection(name, dimensions, metric)` — metric immutable
- `DeleteCollection(name)`
- `ListCollections()`

#### Vector Operations
- `VectorAdd(collection, key, vector, keywords, binary_payload?)`
- `VectorSearch(collection, query_vector, k, search_filter?)`
- `KeywordSearch(collection, keywords, mode, max_distance?)`

#### Search Filter
```go
SearchFilter {
    Keys        []string  // Limit to specific keys (empty = all)
    Keywords    []string  // Keyword filter
    KeywordMode string    // "exact"|"prefix"|"partial"|"levenshtein"
    MaxDistance uint32    // For levenshtein mode
}
```

---

## Critique & Recommendations

### Keyword Field Bloat

**Critique:**
If a user dumps a generic paragraph into the "keywords" field, your `.inv` (inverted index) file will bloat massively, potentially exceeding the size of your actual data.

**Fix:**
Enforce strict keyword validation:
- Limit keywords to characters: `a-z`, `0-9`, `_`, `-`
- Maximum length: 128 characters (128 bytes)

---

### VectorID ↔ Key Mapping Gap

**The Issue:**
HNSW libraries operate internally on `uint64` IDs (e.g., `ID: 5001`), while your KV store uses string keys (e.g., `"doc_alpha"`).

- **Search:** HNSW returns `[5001, 5002]`
- **Retrieval:** The system must fetch the data for these IDs

**The Gap:**
Your current plan stores `Key -> VectorID` in the `data/` shard, but does not define a fast `VectorID -> Key` lookup.

**Critique:**
If HNSW returns `ID: 5001`, how do you find the data? Scanning the entire `shard_001.db` to find which key holds `5001` is inefficient.

**Recommendation:**
Add a lightweight forward index (or "DocID Map") in the `indexes/` folder:

- `doc_map.bin`: An array or map where `Index = VectorID` and `Value = Key` (or file offset).

This enables fast retrieval of keys given a VectorID, significantly reducing retrieval latency.

