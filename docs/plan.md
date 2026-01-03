# WaddleMap: High-Performance Structured Array Database Specification

## 1. Executive Summary
WaddleMap is a specialized, high-performance database designed to store Key-Value pairs satisfying the schema `{ string : array[struct] }`. It is optimized for efficient read/write operations, supporting high-throughput appending, O(1) random access by index, and global search. WaddleMap natively supports partitioning, encryption at rest, and configurable consistency models (from strict ACID to Async Flush).

---

## 2. Data Model & Schema

### 2.1 Logical Schema
The database operates on a sharded, expanding data structure:
- **Schema Notation**: `{ key : [value] }`
- **Data Type**: `{ string : array[struct] }`
- **Key**: String (Variable length, hashed for sharding).
- **Value**: `Vector<Struct>` (Ordered, unbounded length).

### 2.2 The Atomic Structure (The "Item")
Every item in the array is a fixed-size binary struct (size known and pre-defined).
**Constraint**: `Item.ID` must be unique within the scope of a single Key.

```cpp
struct DataItem {
    uint64_t id;        // Unique Identifier (Scope: Key)
    int64_t timestamp;  // For time-series context
    byte payload[N];    // Fixed size payload (pre-defined N)
};
```

---

## 3. Network Interface (Protobuf)
Communication occurs over TCP sockets using Protocol Buffers. This interface supports all core operations: `check_key`, `get_value`, `get_length`, `get_last`, `add`, `search` (global/key), `snapshot`, `update`, `get_keys`, and `get_value_list`.

`waddle_protocol.proto`
```protobuf
syntax = "proto3";

package waddlemap;

service WaddleService {
  rpc Execute (WaddleRequest) returns (WaddleResponse);
}

message WaddleRequest {
  string request_id = 1;
  oneof operation {
    CheckKeyRequest check_key = 2;          // 1. check_key(key)
    GetValueByIndexRequest get_val = 3;     // 2. get_value_by_index(key, index)
    GetLengthRequest get_len = 4;           // 3. get_length(key)
    GetLastValueRequest get_last = 5;       // 4. get_last_value(key)
    AddValueRequest add_val = 6;            // 5. add_value(key, value)
    SearchGlobalRequest search_global = 8;  // 6. search_global(search_param)
    SearchOnKeyRequest search_key = 9;      // 7. search_on_key(key, value)
    SnapshotRequest snapshot = 10;          // 8. snapshot()
    UpdateValueRequest update_val = 7;      // 9. update_value(key, index, value)
  }
}

message WaddleResponse {
  string request_id = 1;
  bool success = 2;
  string error_message = 3;
  oneof result {
    DataItem item = 4;
    uint64 length = 5;
    SearchResult search_results = 6;
  }
}

message DataItem {
  uint64 id = 1;
  int64 timestamp = 2;
  bytes payload = 3;
}

// --- Operation Messages ---

message CheckKeyRequest { string key = 1; }

message GetValueByIndexRequest { 
  string key = 1; 
  uint64 index = 2; 
}

message GetLengthRequest { string key = 1; }
message GetLastValueRequest { string key = 1; }

message AddValueRequest {
  string key = 1;
  DataItem item = 2;
}

message UpdateValueRequest {
  string key = 1;
  uint64 index = 2;
  DataItem item = 3;
}

message SearchGlobalRequest {
  bytes pattern = 1; 
}

message SearchOnKeyRequest {
  string key = 1;
  bytes pattern = 2;
}

message SnapshotRequest { string snapshot_name = 1; }

message SearchResult {
  repeated DataItem items = 1;
}
```

---

## 4. Storage Architecture
WaddleMap uses a Sharded, Paged, Log-Structured architecture to handle massive array growth without performance degradation.

### 4.1 Partitioning (Sharding)
Data is distributed across $N$ physical files (Buckets) using consistent hashing.
- **Shard Logic**: `Bucket_ID = BLAKE3(Key) % Partition_Count`
- **Collision Resistance**: BLAKE3 provides high-speed, cryptographic-grade hashing to ensure uniform distribution. If collisions occur (different keys mapping to same bucket), they are resolved via the B+ Tree Key Index.

### 4.2 File Format (Per Bucket)
Each Bucket File (`waddle_shard_XXX.db`) is structured as follows:

| Section | Description |
| :--- | :--- |
| **Superblock** | Magic bytes ("WADDLE"), Version, Encryption Salt, Snapshot ID. |
| **Key Index Region** | A B+ Tree mapping Key String $\to$ `KeyMetadata`. |
| **Page Heap** | The raw data region divided into fixed-size Pages (e.g., 4KB). |

### 4.3 Internal Structures

**A. KeyMetadata (The "Inode")**
```cpp
struct KeyMetadata {
    char key_string[64];
    uint64_t total_count;    // Current length of the array
    uint32_t root_page_id;   // Pointer to Page Directory
    uint32_t tail_page_id;   // Pointer to the current active append page
    uint16_t tail_offset;    // Byte offset for the next item in the tail page
    BloomFilter filter;      // 512-bit Bloom Filter for O(1) ID uniqueness checks
};
```

**B. Page Directory**
Enables O(1) random access by mapping `Logical_Index` to `Physical_Page_ID`. This acts as a lookup table, preventing the need to traverse a linked list for large arrays.

**C. Data Page**
- **Header**: PageID, Checksum (CRC32), EncryptionNonce.
- **Body**: Contiguous array of `DataItem` binary structures.

---

## 5. Core Operations & Algorithms

### 5.1 System State & Buffering
- **Buffer Pool**: Manages hot data pages in RAM using LRU eviction.
- **Log Policy**: Dual-mode durability support.
    - **Strict Mode (ACID)**: WAL is fsynced before acknowledgement.
    - **Async Flush Mode**: Writes are acknowledged once buffered in the WAL memory; disk flush happens asynchronously (e.g., every 200ms or when buffer fills). Prioritizes throughput over strict instance crash safety.

### 5.2 Unique Identifier Constraint
To enforce that no two items under the same key share an ID:
- **Fast Path**: Check the `KeyMetadata.BloomFilter`. If it returns negative, the ID is unique.
- **Slow Path**: If the filter returns a potential hit, query the `ID_Index` (a specialized hash index of IDs to Page IDs) to confirm if the ID actually exists.

### 5.3 Operation Logic: `update_value(key, index, value)`
1. **Locking**: Acquire WriteLock for the specific bucket.
2. **ID Validation**: Retrieve the item at index to find the `old_id`. If `value.id != old_id`, perform the uniqueness check for the new ID.
3. **Address Lookup**: Calculate `PageIndex = index / ItemsPerPage`. Get the `PhysicalPageID` from the Page Directory.
4. **Commit**: Write the update to the WAL, then modify the page in the Buffer Pool.

---

## 6. Security & ACID

### 6.1 Encryption at Rest
- **AES-256-GCM**: Pages are encrypted individually before being flushed to disk.
- **Nonce/AAD**: The `PageID` is used as Additional Authenticated Data to prevent "page swapping" or "block relocation" attacks.

### 6.2 ACID Properties
- **Atomicity**: WAL-based recovery ensures transactions are all-or-nothing.
- **Consistency**: Page checksums and ID uniqueness enforcement.
- **Isolation**: Bucket-level RWLocks allow concurrent reads but serialized writes per shard.
- **Durability**: 
    - Full durability in **Strict Mode** (Sync WAL).
    - Relaxed durability in **Async Flush Mode** (Background Sync).
---

## 7. Search Implementation: `search_global(pattern)`
- **Parallel Scans**: WaddleMap initiates worker threads across all bucket files.
- **SIMD Acceleration**: Payloads are compared using AVX-512 instructions where available to maximize throughput.
- **Pruning**: Use Bloom Filters in the metadata to skip keys that definitely do not contain the target ID.
