# waddlemap-db

WaddleMap-DB is a high-performance, sharded, append-optimized array database written in Go, with a Python client SDK. It supports efficient random access, vector search, and keyword-based retrieval.

## Build & Run the Server

1. **Build the Go server:**
   ```sh
   go build ./cmd/server
   ```
2. **Run the server:**
   ```sh
   ./server
   ```
   This will create a `waddlemap_db/` directory if it does not exist.

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
   go build ./cmd/server && ./server
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
- See `clients/python/benchmark.py` for a performance benchmark

---
For more details, see the design docs and `clients/python/README.md` for full API reference.

