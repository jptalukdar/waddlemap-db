# Vector Store Mode: Technical Design Document

## 1. Overview

**Objective:**  
Add a vector store mode to support localized data and keyword-based retrieval.

**Core Mechanism:**  
Enable storage of both traditional (binary) and vector data within the same key-value database structure.

---

## 2. Design Architecture

### 2.1 Data Storage

- **HNSW (Hierarchical Navigable Small World):**  
    Utilizes HNSW for storing vector data to enable efficient Approximate Nearest Neighbor (ANN) search.
- **Key-Value DB as Index/Collection:**  
    - Each key represents a collection name.  
    - Each value serves as a pointer/link to the corresponding HNSW store.

### 2.2 Backward Compatibility

- The key-value DB continues to support traditional binary data.
- Users can store binary and vector data side-by-side within the same instance.

### 2.3 Metadata & Data Typing

- **Flagging:** Metadata is added to each key to identify the data type using 3 reserved bits:
    - `000`: Standard binary data
    - `001`: Vector data
    - Other: Reserved for future/special types
- **Structure:**  
    `[key] - [data type (3 bits)] [metadata] [actual data] [address/location of external data]`
- External data refers to the HNSW datastore.
- Metadata includes a list of keyword tokens.

### 2.4 Keyword Indexing

- Supports indexing of specific tokens for faster retrieval.
- Metadata fields support a list of keyword tokens per entry.

### 2.5 Collections

- Collections consist of multiple keys.
- **Constraint:** Data cannot be moved between collections, only copied.
- **Structure:** All vector values in a collection point to the same HNSW datastore.

### 2.6 ANN Support

- Native integration of Approximate Nearest Neighbor (ANN) search capabilities via HNSW.

---

## 3. Finalized Design Decisions

| Aspect                | Decision                                                                 |
|-----------------------|--------------------------------------------------------------------------|
| HNSW Library          | Any battle-tested pure Go or CGo library (no external vector DB).        |
| Vector Dimensions     | Fixed per collection; varies between collections.                        |
| Distance Metrics      | Support for L2 (Euclidean), Cosine, and Inner Product.                   |
| Collection vs Key     | Separate entities; the same name is allowed for both.                    |
| Mixed Data Types      | Keys can store binary AND vector data simultaneously.                    |
| HNSW Storage          | Separate file per collection with HNSW-based sharding.                   |
| Backward Compatibility| Not required (fresh implementation).                                     |

---

## 4. Keyword Specification

- **Case Sensitivity:** Case-insensitive (stored as lowercase).
- **Allowed Characters:** `a-z`, `0-9`, `_` (underscore), `-` (dash).
- **Max Length:** 128 bytes per keyword.
- **Count Limit:** Unlimited keywords per entry.
- **Search Modes (User-Selectable):**
    - `exact`: Exact string match.
    - `prefix`: Prefix matching.
    - `partial`: Substring matching.
    - `levenshtein`: Fuzzy matching with configurable distance.

---

## 5. Core Concept: Data Model

**Definitions:**

*   **Collection:** A group of keys.
*   **Key:** An array of blocks, indexable by an integer. Represents a "document".
*   **Block:** A unit of data containing Primary Data (binary/text) and optional Secondary Data (vector embeddings).

**Hierarchy:**
```
Collection
 └── Key ("doc_alpha")
      ├── Block 0
      |    ├── Index: 0
      │    ├── Keywords: ["intro", "summary"]
      │    ├── Primary: "Executive Summary..."
      │    └── Secondary: [0.1, 0.5...] (Vector)
      ├── Block 1
      |    ├── Index: 1
      │    ├── Keywords: ["finance", "q4"]
      │    ├── Primary: "Q4 Revenue was..."
      │    └── Secondary: [0.8, 0.2...] (Vector)
```

**Search Flow:**
1. User provides query vector + optional filters (keys, keywords).
2. **Filter Step:** If filters are present, limit valid IDs.
3. **Search Step:** HNSW search operates within the filtered set.
4. **Result:** Return matching keys with distances.

---

## 6. Implementation Specifications

### 6.1 Storage Format (On-Disk Encoding)

The storage engine uses a **Variable-Length Header** architecture for forward compatibility, allowing future fields (such as TTL or Transaction IDs) to be added without breaking existing parsers.

#### Entry Layout

```
[Header (Variable Size)] [Key Bytes] [Keywords] [Primary Data] [Secondary Data]
```

#### Header Structure (Current Version: 18 Bytes)

| Offset | Field         | Size    | Description                                                                 |
|--------|--------------|---------|-----------------------------------------------------------------------------|
| 0      | Header Size  | 1 byte  | Total size of the header in bytes (currently 18). Used to locate the Key.   |
| 1      | Flags        | 1 byte  | Bitmask for data types and state (see below).                               |
| 2-3    | Key Len      | 2 bytes | Length of the Key (`uint16`). Max 65KB.                                     |
| 4-7    | Primary Len  | 4 bytes | Length of Primary Data (`uint32`). Max 4GB.                                 |
| 8-11   | Secondary Len| 4 bytes | Length of Secondary Data/Index (`uint32`). Max 4GB.                         |
| 12-13  | Kw Len       | 2 bytes | Length of the serialized Keywords block (`uint16`). Max 65KB.               |
| 14-17  | CRC32        | 4 bytes | Checksum of the entire entry (Header + Key + Data) for integrity.           |
| 18+    | Expansion    | N bytes | Reserved space if Header Size > 18.                                         |

#### Internal Formats

**Keywords Block Encoding**

The `[Keywords]` block (immediately after `[Key Bytes]`) stores the list of keywords used for indexing.

**Format:**
```
[Count (2B)] [Len1 (1B)][Keyword1 Bytes] [Len2 (1B)][Keyword2 Bytes] ...
```
- `Count`: `uint16` indicating the number of keywords.
- `Len`: `uint8` indicating the length of each keyword string (max 128 bytes per keyword).
- `Bytes`: Raw UTF-8 bytes of the keyword.

#### Bitwise Flags (Byte 1)

The Flags byte is divided into **Data Type** (3 bits) and **Status Flags** (5 bits):

| Bit Position | Function    | Logic/Meaning                                 |
|--------------|-------------|-----------------------------------------------|
| 0 - 2        | Data Type   | `000`: Binary Data Only<br>`001`: Vector Data (Binary + Vector)<br>`010-111`: Reserved |
| 3            | Compressed  | `0`: Raw<br>`1`: Compressed (Snappy/Zstd)     |
| 4            | Tombstone   | `0`: Active<br>`1`: Deleted (Ghost entry)     |
| 5 - 7        | Future      | Reserved for future expansion                 |

#### Parsing Logic

1. **Read** Byte 0 (`Header Size`).
2. **Read** Bytes 1-17 to extract flags, lengths, and CRC.
3. **Validate** CRC (optional).
4. **Jump to Key:** Offset = `Header Size`.
5. **Jump to Keywords:** Offset = `Header Size + KeyLen`.
6. **Jump to Primary Data:** Offset = `Header Size + KeyLen + KwLen`.
7. **Jump to Secondary Data:** Offset = `Header Size + KeyLen + KwLen + PrimaryLen`.


**Golang Struct Definitions:**

```go
// Collection Metadata
type CollectionConfig struct {
        Name       string         // Unique collection name
        Dimensions uint32         // Fixed vector dimensions
        Metric     DistanceMetric // "l2" | "cosine" | "ip"
}

// Keyword Entry
type KeywordEntry struct {
        Keywords []string // Normalized, lowercase tokens
        VectorID uint64   // Reference to HNSW vector
        KeyName  string   // Associated key name
}
```


## 7. Directory Structure & Memory

**File System Layout:**
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

**Key Principle:**  
Indexes are derived and rebuildable from the primary data.

**HNSW Memory Management:**
- **Risk:** 5M vectors × 1536 dims ≈ 30GB+ RAM, leading to OOM crashes.
- **Solution:** Use an mmap-backed HNSW library.
    - Graph file is memory-mapped, not fully loaded.
    - OS manages page caching.
    - Enables handling collections larger than available RAM.

---

## 8. Indexing & Algorithms

### 8.1 Keyword Indexing

- **Method:** Trigram (3-gram) Indexing
    - Example: `"finance"` → `["fin", "ina", "nan", "anc", "nce"]`
- **Benefits:**
    - Partial match via set intersection (avoids full scan).
    - Prefix/suffix matching.
    - Levenshtein approximation via trigram overlap.
- **Storage:** Inverted index with postings lists.
    - `trigram → [key1, key2, key3, ...]`

### 8.2 Filtered Search Algorithm

- **Method:** Iterative Filtered Search (Post-filtering during traversal)
- **Keyword Lookup:**
    - If keywords provided → query Inverted Index.
    - Build BitSet of valid key IDs.
- **HNSW Traversal:**
    - Enter graph at entry point.
    - For each candidate node:
        1. Calculate distance.
        2. Check: Is node ID in KeywordBitSet?
                - YES: Add to results heap.
                - NO: Discard from results.
        3. ALWAYS add to traversal queue (to maintain connectivity).
- **Result:** Return top-K from results heap.

---

## 9. Operations & API

### 9.1 Consistency & Durability

- **Challenge:** Keeping KV data and HNSW index files in sync.
- **Strategy:** WAL + Repair-on-Read
    - **WAL (Write-Ahead Log):** Handles atomic writes.
    - **Repair-on-Read:** Detects missing links and cleans up orphans upon load.

### 9.2 Immutability Rules

| Property         | Mutable?                |
|------------------|------------------------|
| Collection Name  | No                     |
| Dimensions       | No (fixed at creation) |
| Distance Metric  | No (fixed at creation) |
| Keys/Vectors     | Yes (add/update/delete)|

### 9.3 API Methods

### 9.3 API Methods

#### Collection Extensions
*   `CompactCollection(collection)` | Defragment the collection. Also removes deleted blocks from the collection. (Time consuming)
*   `ListKeys(collection string) -> []Key` | Lists all keys in the collection.
*   `ContainsKey(collection string, key string) -> bool` | Checks if a key exists in the collection.
*   `Snapshot(collection string) -> SnapshotID` | Creates a point-in-time snapshot.
* `CreateCollection(name, dimensions, metric)`
* `DeleteCollection(name)`
* `ListCollections()`

#### Key & Block Operations
*   `GetBlock(collection string, key string, index int) -> BlockData` | Retrieves the Primary Data for a specific block within a Key array.
*   `GetVector(collection string, key string, index int) -> []float32` | Retrieves the Secondary Data (vector embeddings) for a specific block.
*   `GetKeyLength(collection string, key string) -> int` | Retrieves the length of the array (number of blocks).
*   `GetKey(collection, key) -> []BlockData` | Retrieves all blocks of a specific Key.
*   `DeleteKey(collection, key)` | Removes a Key and all its blocks.
*   `AppendBlock(collection string, key string, data BlockData)` | Appends a new block to the Key array.
*   `UpdateBlock(collection string, key string, index int, data BlockData)` | Updates a specific block within a Key array. If the data doesn't exceed the present block size, run UpdateBlock. Otherwise, run ReplaceBlock.
*   `ReplaceBlock(collection string, key string, index int, data BlockData)` | Replaces a specific block within a Key array. Block will contain the previous index. This will delete the previous block and create a new one.
*   `BatchAppendBlock(collection string, reqs []AppendBlockRequest) -> []bool` | Appends multiple blocks in a single request. Returns success status for each.

#### Search Operations
*   `Search(collection string, query []float32, top_k int, mode string, keywords []string) -> ResultList` | Performs a semantic search across all blocks in the collection filtered by keywords if any. Blank is global.
*   `SearchMoreLikeThis(collection string, key string, index int, top_k int) -> ResultList` | Performs a search using the vector at Key[Index] as the query.
*   `SearchInKey(collection string, key string, query []float32, top_k int) -> ResultList` | Performs a vector search restricted to a single key's array.
*   `BatchSearch()` | Loop Search on multiple queries with same parameters.
*   `KeywordSearch(collection, keywords, match_mode) -> []Key` | Standard keyword-based search.

**Search Filter Struct:**
```go
type SearchFilter struct {
        Keys        []string // Limit to specific keys (empty = all)
        Keywords    []string // Keyword filter
        KeywordMode string   // "exact"|"prefix"|"partial"|"levenshtein"
        MaxDistance uint32   // For levenshtein mode
}
```

---

## 10. Critique & Risk Mitigations

### 10.1 Keyword Field Bloat

- **Risk:** If a user dumps large text blocks into the "keywords" field, the `.inv` (inverted index) file will bloat massively, potentially exceeding the size of the actual data.
- **Mitigation:** Enforce strict keyword validation.
    - Limit keywords to characters: `a-z`, `0-9`, `_`, `-`.
    - Maximum length: 128 bytes per keyword.

### 10.2 VectorID ↔ Key Mapping Gap

- **Risk:** HNSW libraries operate internally on `uint64` IDs (e.g., ID: 5001), while the KV store uses string keys (e.g., "doc_alpha"). Scanning `shard_001.db` to resolve a search result ID 5001 back to "doc_alpha" is inefficient ($O(N)$).
- **Mitigation:** Add a lightweight forward index (DocID Map) in the `indexes/` folder.
    - **File:** `doc_map.bin`
    - **Structure:** Array or Map where Index = VectorID and Value = Key (or file offset).
    - **Result:** Enables $O(1)$ retrieval of keys given a VectorID.