# Copilot Instructions for WaddleMap-DB

## Project Overview
WaddleMap-DB is a high-performance, sharded, append-optimized array database written in Go, with a Python client SDK. It stores `{string: array[struct]}` mappings, supporting efficient random access, vector search, and keyword-based retrieval. Data is persisted in sharded files with ACID or async durability, and accessed via a Protobuf-based TCP API.

## Architecture & Key Components
- **Go Server** (`cmd/server`, `internal/`):
  - **Network Layer**: Handles TCP, Protobuf, and request routing via Go channels.
  - **Transaction Manager**: Validates and dispatches requests, manages concurrency.
  - **Storage Engine**: Sharded, log-structured, with per-bucket locking, WAL, and in-memory indexes.
  - **Types**: Shared structs for requests, responses, and config.
- **Python Client** (`clients/python/`):
  - Serializes user structs, wraps in Protobuf, manages socket connections, and exposes a user-friendly API.
- **Protobuf API** (`proto/waddle_protocol.proto`):
  - Defines all network operations (add, get, search, update, etc.).

## Developer Workflows
- **Build server**: `go build ./cmd/server`
- **Run server**: `./server` (creates `waddlemap_db/` if missing)
- **Test (Go)**: `go test ./internal/...`
- **Test (Python client)**: Run scripts in `clients/python/`
- **Regenerate Protobufs**: 
  - Go: `protoc --go_out=. --go-grpc_out=. proto/waddle_protocol.proto`
  - Python: `python -m grpc_tools.protoc -I./proto --python_out=clients/python --grpc_python_out=clients/python proto/waddle_protocol.proto`

## Project-Specific Patterns & Conventions
- **All inter-module communication** uses Go channels and context structs (`RequestContext`, `ResponseContext`).
- **Sharding**: Keys are mapped to buckets using BLAKE3 hash; see `plan.md` for details.
- **Vector search**: Uses HNSW index per collection; see `internal/storage/`.
- **Keyword search**: Inverted index per collection.
- **Strict vs Async durability**: Controlled by config; affects WAL/fsync policy.
- **Client struct packing**: Python users must define fixed-size structs for payloads.
- **Error handling**: All network errors are returned in the Protobuf response; client raises exceptions on failure.

## Key Files & Directories
- `architecture.md`, `plan.md`, `low_level_design.md`: High-level and detailed design docs
- `cmd/server/main.go`: Server entrypoint
- `internal/network/server.go`: TCP/Protobuf server logic
- `internal/transaction/manager.go`: Request dispatch, validation
- `internal/storage/`: Sharding, persistence, vector/keyword search
- `proto/waddle_protocol.proto`: Protobuf API
- `clients/python/waddle_client.py`: Python SDK

## Examples
- **Add value (Python)**:
  ```python
  client.append_block("mycol", "mykey", b"payload", vector=[0.1,0.2], keywords=["foo"])
  ```
- **Get block (Python)**:
  ```python
  client.get_block("mycol", "mykey", 0)
  ```
- **Search (Python)**:
  ```python
  client.search("mycol", [0.1,0.2], top_k=5)
  ```

## Tips
- Always match struct size to `payload_size` in config.
- Use the Python SDK for all client operations; do not send raw Protobufs manually.
- For new operations, update both Go and Python Protobufs, and add handlers in both server and client.

---
For more details, see the design docs and referenced source files. If anything is unclear or missing, please provide feedback for improvement.
