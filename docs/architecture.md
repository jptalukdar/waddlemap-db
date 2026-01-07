# WaddleDB Architecture Documentation

WaddleDB is an embedded, persistent vector database designed for managing structured documents with semantic search capabilities. It combines traditional key-value storage with vector similarity search, allowing users to store, retrieve, and search data at the block level within documents.

## 1. System Overview

The system is built as a modular Go application consisting of a core server and client libraries (Python, Go). The architecture emphasizes concurrency, data safety via WAL (Write-Ahead Logging), and efficient vector retrieval using HNSW (Hierarchical Navigable Small World) graphs.

### Core Modules

*   **Network Layer**: Handles incoming connections (Socket) and converts requests into internal transaction objects.
*   **Transaction Manager**: Orchestrates operations, ensuring ACID properties (where applicable) and routing requests to the appropriate storage subsystems.
*   **Storage Manager**: The foundational layer handling disk I/O, caching, and basic key-value operations.
*   **Vector Manager**: An extension of the Storage Manager that handles vector embeddings, HNSW indices, and collection management.

---

## 2. Data Model

WaddleDB uses a hierarchical data model designed for structured documents:

### 2.1 Hierarchy
1.  **Collection**: A logical grouping of Keys (e.g., "Articles", "Users"). Each collection has a defined vector dimension and distance metric (L2, Cosine, IP).
2.  **Key**: Represents a single document (e.g., "doc_123"). A Key maps to an ordered array of **Blocks**.
3.  **Block**: The fundamental unit of data.
    *   **Index**: Position in the Key's array (0-indexed).
    *   **Primary Data**: The actual content (Text, JSON, Blob).
    *   **Secondary Data**: Vector embedding (float32 array) representing the primary data.
    *   **Keywords**: List of tags or keywords for filtering.

### 2.2 Conceptual View
```text
Collection ("Wiki")
 └── Key ("page_alpha")
      ├── Block 0
      │    ├── Primary: "User gives a custom struct..."
      │    ├── Secondary: [0.12, -0.5, ...] (Vector)
      │    └── Keywords: ["intro", "struct"]
      └── Block 1
           ├── Primary: "The same struct needs to be..."
           ├── Secondary: [0.8, 0.2, ...] (Vector)
           └── Keywords: ["protobuf", "network"]
```

---

## 3. Storage Architecture

The storage layer is a hybrid system combing a persistent KV store and memory-mapped vector indices.

### 3.1 Primary Storage (KV)
*   Stores the raw block data (Primary, Vector, Keywords).
*   Keys in the underlying KV store are composed as `Collection:Key`.
*   Supports efficient append, get-by-index, and range retrieval.

### 3.2 Vector Storage (HNSW)
*   **Index Structure**: Uses a custom **Binary HNSW Format** (`HNSWV001`).
*   **Persistence**:
    *   Indices are saved to disk in a compact binary format containing a header, node table, vector data, and neighbor graphs.
    *   **WAL (Write-Ahead Log)**: All write operations (Append, optional Delete) are logged to a `vector.wal` ensures crash recovery.
    *   **Checkpoints**: Periodic checkpoints flush the in-memory HNSW graphs to disk (binary format) and truncate the WAL.

### 3.3 Batch & Concurrency
*   **Batch Ingestion**: Supports specific `BatchAppendBlock` operations. These operations group WAL writes and storage commits to maximize throughput.
*   **Durability**: For single writes, the HNSW index may be flushed immediately or on a trigger. For batch writes, flushing is deferred to a Checkpoint for performance, relying on WAL for durability.

---

## 4. API & Operations

The system exposes a rich set of RPC-style operations.

### 4.1 Basic Operations
*   **`AppendBlock(collection, key, block)`**: Adds a block to the end of a key's array.
*   **`BatchAppendBlock(collection, requests[])`**: Optimized high-throughput ingestion.
*   **`GetBlock(collection, key, index)`**: Retrieves the full block (Primary + Vector).
*   **`GetRelativeBlocks(collection, key, center_index, before, after)`**: Retrieves a window of blocks around a specific index. Useful for fetching context (e.g., in RAG applications).
*   **`GetKeyLength(collection, key)`**: Returns the number of blocks in a document.
*   **`DeleteKey(collection, key)`**: Removes a document and its vectors.

### 4.2 Vector Operations
*   **`Search(collection, query, top_k, keywords)`**: Global semantic search across the entire collection. Supports filtering by keywords.
*   **`SearchInKey(collection, key, query, top_k)`**: Scoped semantic search within a single document (Key).
*   **`SearchMoreLikeThis(collection, key, index, top_k)`**: Uses the vector of an existing block as the query for a global search.
*   **`GetVector(collection, key, index)`**: Retrieves only the vector embedding for analysis/re-ranking.

### 4.3 Management Operations
*   **`CreateCollection(name, dims, metric)`**: Initialize a new vector space.
*   **`Snapshot(collection)`**: Creates a point-in-time snapshot of the index.
*   **`CompactCollection(collection)`**: Triggers defragmentation (implementation pending).

---

## 5. Client Protocol

Communication is handled via a flexible Protobuf protocol defined in `waddle_protocol.proto`.

*   **Request/Response**: All interactions use a unified `WaddleRequest` and `WaddleResponse` envelope.
*   **OneOf Polymorphism**: Requests contain a `oneof` field acting as a union type for specific operation arguments (e.g., `AppendBlockRequest`, `SearchRequest`).
*   **Clients**:
    *   **Go Client**: Native usage via internal packages.
    *   **Python Client**: Connects via Socket, exposing a pythonic API for data science and AI workflows.

---

## 6. Storage Layout

The database manages data persistence through a structured directory layout and specialized binary formats.

### 6.1 Directory Structure
```text
waddlemap_db/
├── data/                  # Primary Key-Value Store
│   └── ...                # KV Store files
├── vector.wal             # Write-Ahead Log (Global)
└── indexes/               # Vector Indices
    └── <CollectionName>/  # Per-Collection Directory
        ├── vectors.hnsw   # HNSW Vector Index (Binary)
        ├── keywords.inv   # Inverted Keyword Index
        ├── doc_map.bin    # Forward Index (VectorID -> Key Mapping)
        └── meta.json      # Collection Metadata
```

### 6.2 File Formats

#### HNSW Index (`vectors.hnsw`)
A custom binary format optimized for memory mapping (mmap) and fast loading. It consists of four contiguous sections:

1.  **Header (64 bytes)**:
    *   Magic: `HNSWV001`
    *   Metadata: Dimensions, Metric (L2/Cosine/IP), Node Count, Entry Point ID, Max Level.
    *   Configuration: Max Connections (M).

2.  **Node Table**:
    An array of fixed-size records for every node in the graph.
    *   `ID` (8 bytes)
    *   `Level` (4 bytes)
    *   `VectorOffset` (4 bytes): Pointer to vector data.
    *   `NeighborOffset` (4 bytes): Pointer to neighbor list.
    *   `NeighborCount` (4 bytes): Total count of neighbors across all levels.

3.  **Vector Data**:
    A contiguous block containing raw float32 vectors for all nodes.

4.  **Neighbor Lists**:
    Adjacency lists for the HNSW graph layers. Stored as:
    *   `[LevelCount]` (2 bytes)
    *   For each level:
        *   `[NeighborCount]` (2 bytes)
        *   `[NeighborID_1, NeighborID_2, ...]` (8 bytes each)

#### Write-Ahead Log (`vector.wal`)
Used for crash recovery, the WAL records all mutation events before they are applied to the in-memory index or flushed to disk.
*   **Format**: Sequential GOB-encoded stream.
*   **Entry Structure**: `{Timestamp, OpType, Collection, Key, VectorID, Vector, Keywords, PrimaryData}`.
*   **Operations**: `Add`, `Delete`, `Update`.
