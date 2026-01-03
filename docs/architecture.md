user gives a custom struct to the database. This struct is used for values in the database. 
The same struct needs to be converted to protobuf struct for network communication by the client.

There is a core go module, that implements the database. It has socket which handles the connections and sends it to a transaction manager via channels.
Transaction manager handles the transactions and sends it to the storage manager via channels.
Storage manager handles the actual storage of the data.

The program runs a init module on the startup that checks if the database is initialized or not. If not, it creates a directory waddlemap_db to hold the data. It also creates a config file to store the configuration of the database.

The init module initializes all modules and starts the server. It starts separate threads for all the managers. Each of them communicates via go channels. 

Every transaction is handled via a go-routine. 

Create a python client that connects to the db via socket.
It performs all the operations on the db.





add operations for vector store

// 1. check_key(key)
// 2. get_value_by_index(key, index) // Returns primary data
// 3. get_length(key) // Returns a tuple of (length_primary data , length secondary data)
// 4. get_last_value(key) // Returns primary data
// 5. add_value(key, value) // Add a block to the data.
// 6. search_global(search_param) // Returns all the keys that match the search param
// 7. search_on_key(key, value)
// 8. snapshot()
// 9. update_value(key, index, value)



check_key -> check if key exists

model each key as a document 
The array is a series of chunks of data. One can add to it, and can remove from it. Removing won't delete the document. It will just remove the chunk from the array by making it a blank chunk. This causes fragmentation. 

Add defragmentation function to remove the fragmentation caused by deleted data.

get_value_by_index -> returns primary value
get_vector_by_index -> returns secondary data in form of the vector embedddings. 
get_length -> returns the length of the array identified by the keys.
vector_search_key(top k: default array length) -> perform a vector search on the entire array for a single key
vector_search_global(top k: int , return mode: key | block, filtered_keywords: list of keywords) -> perform a vector search on the entire database , returns the list of keys or blocks that match based on the return mode. If return mode is key, it returns the list of keys that match. If return mode is block, it returns the list of blocks that match.



A giant clarification

Collection -> A group of keys
Key -> Array of blocks indexible by an integer
Each block has a primary data and an optional secondary data (vector embeddings). 



Operation on collection

1. check if key exist on collection.


Operation on a specific key: array
1. check if index exist on array.
2. get_value_by_index
3. get_length -> get the length of the array.
4. get_size -> in bytes calculate the size of the entire array.
4. get_last_value -> get the last block of the array.
5. add_value -> add a block to the array.
6. remove_value -> make the block a zero vector. skip from any operation.
7. update_value -> add then remove



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


GetBlock(collection string, key string, index int) -> BlockData | Retrieves the Primary Data for a specific block within a Key array.

GetVector(collection string, key string, index int) -> []float32 | Retrieves the Secondary Data (vector embeddings) for a specific block within a Key array.

GetKeyLength(collection string, key string) -> int | Retrieves the length of the array.

Search(collection string, query []float32, top_k int, mode string, keywords []string) -> ResultList | Performs a semantic search across all blocks in the collection filtered by keywords if any. Blank is global

SearchMoreLikeThis(collection string, key string, index int, top_k int) -> ResultList  | Performs a search using the vector at Key[Index] as the query.

SearchInKey(collection string, key string, query []float32, top_k int) -> ResultList | Performs a vector search restricted to a single key's array.

DeleteKey(collection, key) | Removes a Key and all its blocks.

GetKey(collection, key) -> []BlockData | Retrieves all blocks of a specific Key.

BatchSearch() -> Loop Search on multiple queries with same parameters.

KeywordSearch(collection, keywords, match_mode) -> []Key

CompactCollection(collection) | Defragment the collection. Also removes deleted blocks from the collection. Time consuming

AppendBlock(collection string, key string, data BlockData) | Appends a new block to the Key array.

ListKeys(collection string) -> []Key | Lists all keys in the collection.

ContainsKey(collection string, key string) -> bool | Checks if a key exists in the collection.

Snapshot(collection string) -> SnapshotID

UpdateBlock(collection string, key string, index int, data BlockData) | Updates a specific block within a Key array. If the data doesn't exceed the present block size, run UpdateBlock. Otherwise, run ReplaceBlock. This will update the vector embeddings if present. 

ReplaceBlock(collection string, key string, index int, data BlockData) | Replaces a specific block within a Key array. Block will contain the previous index. This will delete the previous block and create a new one. This will also update the vector embeddings if present.
