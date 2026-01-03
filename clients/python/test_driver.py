import time
import sys
from waddle_client import WaddleClient


def main():
    print("Starting WaddleMap Full Operation Test...")
    client = WaddleClient()

    # Create test collection
    collection_name = "test_collection"
    try:
        client.delete_collection(collection_name)
    except:
        pass

    collection = client.create_collection(collection_name, dimensions=4)

    try:
        key = "sensor_X"
        primary_1 = "Data_Block_1"
        primary_2 = "Data_Block_2"
        primary_upd = "Data_Block_U"
        vector_1 = [0.1, 0.2, 0.3, 0.4]
        vector_2 = [0.2, 0.3, 0.4, 0.5]

        # 1. Append Block 1
        print("Test 1: Append Block 1...", end="")
        collection.append_block(key, primary_1, vector=vector_1, keywords=["block1"])
        print(" PASS")

        # 2. Append Block 2
        print("Test 2: Append Block 2...", end="")
        collection.append_block(key, primary_2, vector=vector_2, keywords=["block2"])
        print(" PASS")

        # 3. Get Block (Index 0)
        print("Test 3: Get Block (idx 0)...", end="")
        block = collection.get_block(key, 0)
        if block.primary == primary_1:
            print(" PASS")
        else:
            raise Exception(f"Expected {primary_1}, got {block.primary}")

        # 4. Get Block (Index 1)
        print("Test 4: Get Block (idx 1)...", end="")
        block = collection.get_block(key, 1)
        if block.primary == primary_2:
            print(" PASS")
        else:
            raise Exception(f"Expected {primary_2}, got {block.primary}")

        # 5. Vector Search
        print("Test 5: Vector Search...", end="")
        results = collection.search(vector_1, top_k=2)
        if len(results) >= 1:
            print(f" PASS (Found {len(results)} matches)")
        else:
            raise Exception("Search failed or no results")

        # 6. Keyword Search
        print("Test 6: Keyword Search...", end="")
        keys = collection.keyword_search(["block1"])
        if key in keys:
            print(f" PASS (Found key)")
        else:
            raise Exception(f"Key {key} not found in keyword search")

        # 7. List Keys
        print("Test 7: List Keys...", end="")
        keys = collection.list_keys()
        if key in keys:
            print(f" PASS (Found {len(keys)} keys)")
        else:
            raise Exception(f"Key {key} not found in key list: {keys}")

        # 8. Contains Key
        print("Test 8: Contains Key...", end="")
        if collection.contains_key(key):
            print(" PASS")
        else:
            raise Exception("Contains key check failed")

        # 9. Batch Append
        print("Test 9: Batch Append...", end="")
        items = [
            {
                "key": "batch_1",
                "primary": "Batch Item 1",
                "vector": [0.5, 0.6, 0.7, 0.8],
                "keywords": ["batch"],
            },
            {
                "key": "batch_2",
                "primary": "Batch Item 2",
                "vector": [0.6, 0.7, 0.8, 0.9],
                "keywords": ["batch"],
            },
        ]
        collection.batch_append_blocks(items)
        print(" PASS")

    except Exception as e:
        import traceback

        traceback.print_exc()
        print(f"\n[FAIL] Client Test Error: {e}")
        sys.exit(1)
    finally:
        client.close()


if __name__ == "__main__":
    main()
