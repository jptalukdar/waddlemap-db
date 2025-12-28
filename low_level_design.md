# WaddleMap Low-Level Design (LLD)

This document outlines the internal module design, data flow, and struct definitions for the WaddleMap database, based on the high-level architecture.

## 1. System Overview & Module Hierarchy

 The system consists of a centralized Go server (`waddledb`) and a Python Client SDK. The server follows a pipelined architecture using Go Channels for inter-module communication.

### Module Map
1.  **Initialization (`main`)**: Startup, Config, Dependency Injection.
2.  **Network Layer (`network`)**: TCP Handling, Protobuf Marshaling.
3.  **Transaction Manager (`transaction`)**: Request validation, Isolation, Scheduling.
4.  **Storage Engine (`storage`)**: File I/O, Sharding, Caching, WAL.
5.  **Client SDK (`python`)**: User interface.

---

## 2. Shared Data Structures (`internal/types`)

To facilitate communication between modules via channels, we define internal context wrappers.

```go
// Helper struct to define the user's fixed-size payload
type DBSchemaConfig struct {
    PayloadSize int    // Size of the struct in bytes (N)
    DataPath    string // Path to waddlemap_db
    SyncMode    string // "strict" or "async"
}

// Passed from Network -> Transaction Manager
type RequestContext struct {
    ReqID      string
    Operation  ProtocolMethod // Enum: GET, PUT, SCAN, etc.
    Params     interface{}    // The unmarshaled params (Key, Index, Item)
    RespChan   chan ResponseContext // Channel to send the result back to Network routine
}

// Passed from Transaction Manager -> Network
type ResponseContext struct {
    ReqID   string
    Success bool
    Data    interface{} // Resulting Item, Length, or Error
    Error   error
}
```

---

## 3. Module Details

### 3.1 Initialization Module (`cmd/server`)

**Role**: Bootstrap the application.
**Responsibilities**:
1. Check for existence of `./waddlemap_db`. If missing, create it.
2. Create/Load `config.yaml` (Defines `PayloadSize`, `Port`).
3. Initialize `StorageManager`.
4. Initialize `TransactionManager` (passing `StorageManager` ref).
5. Start `NetworkServer` (passing `TransactionManager` ref).
6. Handle OS signals (SIGINT, SIGTERM) for graceful shutdown.

### 3.2 Network Layer (`internal/network`)

**Role**: Interface with external clients.
**Responsibilities**:
1. Listen on TCP Port (default: 6969).
2. Accept incoming connections.
3. Spawns a **Goroutine per connection**.
    - Reads raw bytes -> Unmarshals Protobuf.
    - Wraps into `RequestContext`.
    - Sends to `TransactionManager.InputChan`.
    - Waits on `RequestContext.RespChan`.
    - Marshals `ResponseContext` -> Protobuf -> Writes to socket.

**Concurrency**: High (One goroutine per client).

### 3.3 Transaction Manager (`internal/transaction`)

**Role**: Orchestrate request flow and enforce consistency.
**Structure**:
```go
type TxManager struct {
    InputChan   chan RequestContext
    StorageMgr  *storage.Manager
}
```
**Responsibilities**:
1. **Dispatcher Loop**: Runs a pool of worker goroutines consuming `InputChan`.
2. **Validation**: Checks if request parameters are valid (e.g., Key length).
3. **Routing**: Calls specific methods on `StorageManager` (e.g., `storage.Append(key, item)`).
4. **Error Handling**: Catches storage errors and formats them for the response.

*Note*: In this design, the Transaction Manager primarily acts as a scheduler/router. Actual locking happens closer to the data in the Storage Manager to allow fine-grained bucket parallelism.

### 3.4 Storage Manager (`internal/storage`)

**Role**: Data persistence and retrieval.
**Structure**:
```go
type Manager struct {
    Buckets     map[uint32]*Bucket // Map of BucketID -> Bucket Instance
    GlobalIndex *BTree              // Global Key -> Bucket mapping (if dynamic) or Hash function
}

type Bucket struct {
    FileID      uint32
    FileHandle  *os.File
    WriteLock   sync.RWMutex       // Read-Write Lock for this specific shard
    MemTable    []DataItem         // Write Buffer (for Async Flush)
}
```

**Responsibilities**:
1. **Sharding**: Hash Key -> `BucketID`.
2. **Locking**: Acquire `RLock` for Reads, `Lock` (Write) for Appends/Updates on the specific Bucket.
3. **Operation Implementation**:
    - `Get(key, index)`: Seek to offset, Read.
    - `Append(key, value)`:
        - Check bloom filter (in memory).
        - If unique, append to WAL.
        - Append to MemTable/Page.
4. **Flushing**:
    - If `AsyncMode`: A background Ticker goroutine iterates buckets and calls `fsync`.
    - If `StrictMode`: Call `fsync` immediately after Write.

---

## 4. Client SDK (`clients/python`)

**Role**: Developer-friendly wrapper.

**Responsibilities**:
1. **Struct Packing**:
    - User defines a Python `class` using `ctypes` or `struct` module.
    - Client SDK serializes this class into `bytes` before sending.
2. **Protobuf Wrapping**:
    - Wraps the serialized bytes into `WaddleRequest` proto.
3. **Socket Management**:
    - Maintains persistent TCP connection.
    - Auto-reconnect logic.

**Example Usage Flow**:
```python
# User defines valid structure
class MySensorData(Structure):
    _fields_ = [("temp", c_float), ("id", c_int)]

# Client Usage
client = WaddleClient(host="localhost", payload_size=8)
data = MySensorData(23.5, 101)
client.add_value("sensor_1", data)
```

---

## 5. Flow Diagram (Add Value)

1. **Client**: Serializes `MySensorData` -> `bytes`. Creates `AddValueRequest`. Sends over TCP.
2. **Network**: Receives bytes. Decodes Proto. Creates `RequestContext`. Sends to `InputChan`.
3. **TxManager**: Reads `InputChan`. Identifies `AddValue`. Calls `Storage.Append(key, data)`.
4. **Storage**:
    - Hashes Key -> Bucket 4.
    - Locks Bucket 4.
    - Checks Uniqueness (Bloom/Index).
    - Writes to WAL.
    - Updates In-Memory Page.
    - Unlocks.
    - Returns `Success`.
5. **TxManager**: Sends `Success` to `RespChan`.
6. **Network**: Reads `RespChan`. Encodes `WaddleResponse`. Sends TCP.
