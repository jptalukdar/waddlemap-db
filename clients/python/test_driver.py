import time
import sys
from waddle_client import WaddleClient

def main():
    print("Starting WaddleMap Full Operation Test...")
    client = WaddleClient()
    
    try:
        key = "sensor_X"
        payload_1 = b"Data_Block_1"
        payload_2 = b"Data_Block_2"
        payload_upd = b"Data_Block_U" # Same size as 1

        # 1. Add Value 1
        print("Test 1: Add Value 1...", end="")
        resp = client.add_value(key, payload_1)
        if not resp.success: raise Exception(resp.error_message)
        print(" PASS")

        # 2. Add Value 2
        print("Test 2: Add Value 2...", end="")
        resp = client.add_value(key, payload_2)
        if not resp.success: raise Exception(resp.error_message)
        print(" PASS")

        # 3. Get Length
        print("Test 3: Get Length...", end="")
        resp = client.get_length(key)
        if resp.success and resp.length == 2:
            print(f" PASS (Len: {resp.length})")
        else:
            raise Exception(f"Expected 2, got {resp.length}")

        # 4. Get Value (Index 0)
        print("Test 4: Get Value (idx 0)...", end="")
        resp = client.get_value(key, 0)
        if resp.success and resp.item.payload == payload_1:
             print(" PASS")
        else:
             raise Exception(f"Expected {payload_1}, got {resp.item.payload}")

        # 5. Update Value (Index 0)
        print("Test 5: Update Value (idx 0)...", end="")
        resp = client.update_value(key, 0, payload_upd)
        if not resp.success: raise Exception(resp.error_message)
        
        # Verify Update
        resp = client.get_value(key, 0)
        if resp.item.payload == payload_upd:
            print(" PASS (Verified Update)")
        else:
            raise Exception("Update verification failed")

        # 6. Global Search
        print("Test 6: Global Search...", end="")
        resp = client.search_global(b"Block_2")
        if resp.success and len(resp.search_results.items) >= 1:
            print(f" PASS (Found {len(resp.search_results.items)} matches)")
        else:
             raise Exception("Search failed or no results")

        # 7. Snapshot
        print("Test 7: Snapshot...", end="")
        resp = client.snapshot("test_snap_1")
        if resp.success:
            print(" PASS")
        else:
             raise Exception(resp.error_message)

        # 8. Get Keys
        print("Test 8: Get Keys...", end="")
        resp = client.get_keys()
        if resp.success:
            keys = resp.key_list.keys
            if key in keys:
                print(f" PASS (Found {len(keys)} keys)")
            else:
                raise Exception(f"Key {key} not found in key list: {keys}")
        else:
            raise Exception(resp.error_message)

        # 9. Get Value List
        print("Test 9: Get Value List...", end="")
        # We updated index 0 to payload_upd. Let's add another value to check list.
        payload_3 = b"Data_Block_3"
        client.add_value(key, payload_3)
        
        resp = client.get_value_list(key)
        if resp.success:
            items = resp.value_list.items
            # We expect: [0]=payload_upd, [1]=payload_2, [2]=payload_3
            # Note: Index 0 was updated. Index 1 was payload_2. Index 2 is payload_3.
            if len(items) >= 3:
                print(f" PASS (Retrieved {len(items)} items)")
            else:
                 raise Exception(f"Expected >=3 items, got {len(items)}")
        else:
            raise Exception(resp.error_message)

    except Exception as e:
        import traceback
        traceback.print_exc()
        print(f"\n[FAIL] Client Test Error: {e}")
        sys.exit(1)
    finally:
        client.close()

if __name__ == "__main__":
    main()
