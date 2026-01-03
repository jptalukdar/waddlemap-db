# WaddleMap Python Client

## Overview

The WaddleMap Python client provides a simple, object-oriented interface to interact with the WaddleMap database server.

## Installation

```bash
# Ensure you have the protobuf files generated
# No additional installation needed - just import waddle_client
# Or install using the package (once distributed):
# pip install waddle-client
```

## Quick Start

```python
from waddle_client import WaddleClient

# Connect to the server
client = WaddleClient(host='localhost', port=6969)

# Create a collection
collection = client.create_collection(
    name="my_collection",
    dimensions=384,  # Vector dimensions
    metric="l2"      # Distance metric: "l2", "cosine", etc.
)

# Add data with vectors
collection.append_block(
    key="doc1",
    primary="This is my document text",
    vector=[0.1, 0.2, 0.3],  # Example vector (use real embedding)
    keywords=["tag1", "tag2"]
)

# Search by vector
results = collection.search(
    vector=[0.1, 0.2, 0.3],
    top_k=10
)

# Access results
for result in results:
    print(f"Key: {result.key}")
    print(f"Distance: {result.distance}")
    print(f"Content: {result.block.primary}")
    print(f"Keywords: {result.block.keywords}")

# Close connection
client.close()
```

## API Reference

### WaddleClient

The main client class for connecting to WaddleMap server.

#### Constructor

```python
client = WaddleClient(host='localhost', port=6969)
```

#### Methods

##### `create_collection(name, dimensions, metric="l2")`
Creates a new collection and returns a Collection object.

**Parameters:**
- `name` (str): Collection name
- `dimensions` (int): Vector dimensions
- `metric` (str): Distance metric ("l2", "cosine", etc.)

**Returns:** `Collection` object

##### `collection(name)`
Gets a reference to an existing collection.

**Parameters:**
- `name` (str): Collection name

**Returns:** `Collection` object

##### `delete_collection(name)`
Deletes a collection by name.

**Parameters:**
- `name` (str): Collection name

##### `list_collections()`
Lists all collections in the database.

**Returns:** List of collection info

##### `close()`
Closes the connection to the server.

---

### Collection

Represents a WaddleMap collection with all its operations.

#### Methods

##### `append_block(key, primary, vector=None, keywords=None)`
Appends a block to a key in this collection.

**Parameters:**
- `key` (str): The key to append to
- `primary` (str): Primary text/data content
- `vector` (list[float], optional): Vector embedding
- `keywords` (list[str], optional): Keywords for search

##### `batch_append_blocks(items)`
Batch append multiple blocks to this collection.

**Parameters:**
- `items` (list[dict]): List of dicts with keys: 'key', 'primary', 'vector', 'keywords'

**Example:**
```python
items = [
    {
        'key': 'doc1',
        'primary': 'First document',
        'vector': [0.1, 0.2, ...],
        'keywords': ['tag1']
    },
    {
        'key': 'doc2',
        'primary': 'Second document',
        'vector': [0.3, 0.4, ...],
        'keywords': ['tag2']
    }
]
collection.batch_append_blocks(items)
```

##### `get_block(key, index)`
Gets a specific block from a key in this collection.

**Parameters:**
- `key` (str): The key
- `index` (int): Block index (0-based)

**Returns:** `BlockData` object

##### `delete_key(key)`
Deletes a key and all its blocks from this collection.

**Parameters:**
- `key` (str): The key to delete

##### `list_keys()`
Lists all keys in this collection.

**Returns:** List of key names

##### `contains_key(key)`
Checks if a key exists in this collection.

**Parameters:**
- `key` (str): The key to check

**Returns:** `bool`

##### `search(vector, top_k=10, keywords=None, mode="global")`
Performs vector search in this collection.

**Parameters:**
- `vector` (list[float]): Query vector
- `top_k` (int): Number of results to return
- `keywords` (list[str], optional): Optional keyword filters
- `mode` (str): Search mode ("global" or "local")

**Returns:** List of search results

**Example:**
```python
results = collection.search([0.1, 0.2, 0.3, ...], top_k=5)
for result in results:
    print(f"{result.key}: {result.distance}")
```

##### `keyword_search(keywords, mode="exact")`
Performs keyword search in this collection.

**Parameters:**
- `keywords` (list[str]): List of keywords to search for
- `mode` (str): Search mode ("exact" or other modes)

**Returns:** List of keys matching the keywords

##### `delete()`
Deletes this entire collection.





## Examples

See the following files for complete examples:
- `../../tests/feature_test.py` - Full feature test covering most API methods
- `../../tests/semantic_search_test.py` - Semantic search example
- `../../tests/evaluation_test.py` - Evaluation benchmark
- `../../tests/comparison_test.py` - Performance comparison with ChromaDB

## Notes

- All operations raise exceptions on error (no need to check `success` field)
- The client maintains a persistent TCP connection to the server
- Always call `client.close()` when done to properly close the connection
- Collections use 0-based indexing for block indices
