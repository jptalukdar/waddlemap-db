# waddlemap-db

WaddleMap-DB is a high-performance, sharded, append-optimized array database written in Go, with a Python client SDK. It supports efficient random access, vector search, and keyword-based retrieval.

## Build & Run the Server

1. **Build the Go server:**
   ```sh
   go build -o waddle-server.exe ./cmd/server
   ```
2. **Run the server:**
   ```sh
   # Standard mode
   .\waddle-server.exe

   # Quiet mode (errors only)
   .\waddle-server.exe -quiet
   ```
   This will create a `data/` directory if it does not exist.

## Performance Benchmarks

Comparisons run against ChromaDB (local persistent mode) on the same hardware.

**Ingestion Speed (552 chunks, 384d dim):**
- **WaddleDB:** 0.22s (~2500 chunks/s) 🚀
- **ChromaDB:** 0.59s (~935 chunks/s)

**Search Latency (Avg):**
| Scenario | WaddleDB | ChromaDB |
|----------|----------|----------|
| Single Passage | 0.77 ms | 2.80 ms |
| Multi Passage | 0.81 ms | 2.40 ms |
| No Answer | 0.68 ms | 2.50 ms |

WaddleDB demonstrates **~2.7x faster ingestion** and **~3x lower search latency** compared to ChromaDB in this benchmark.

## Python Client Installation

The Python client is located in `clients/python/`. No installation is required if you run scripts from this directory. Ensure you have Python 3.7+ and `protobuf` installed:

```sh
pip install protobuf
```

If you modify the Protobuf schema, regenerate the Python files:

```sh
python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/waddle_protocol.proto
```

## Using the Python Client

You can use the client directly in your Python scripts:

```python
from waddle_client import WaddleClient

# Connect to the server
client = WaddleClient(host='localhost', port=6969)

# Create a collection
collection = client.create_collection('mycol', dimensions=128)

# Append a block
collection.append_block('mykey', 'payload', vector=[0.1, 0.2], keywords=['foo'])

# Retrieve a block
block = collection.get_block('mykey', 0)
print(block.primary)

# Search
results = collection.search([0.1, 0.2], top_k=5)
for res in results:
    print(res.key, res.distance)

client.close()
```

## Quick Run Example

1. **Start the server:**
   ```sh
   go build -o waddle-server.exe ./cmd/server && .\waddle-server.exe
   ```
2. **Run a test script:**
   ```sh
   cd clients/python
   python test_block_store.py
   ```
   This will run a basic CRUD test using the Python client.

## More Examples
- See `clients/python/test_block_store.py` for a basic test
- See `tests/semantic_search_test.py` for a semantic search example
- See `tests/comparison_test.py` for the performance benchmark

---
For more details, see the design docs and `clients/python/README.md` for full API reference.

