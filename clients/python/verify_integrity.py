import os
import hashlib
from waddle_client import WaddleClient

def main():
    HOST = 'localhost'
    PORT = 6969
    FILE_SIZE = 1 * 1024 * 1024 # 1 MB
    CHUNK_SIZE = 16 * 1024     # 16 KB
    SOURCE_FILE = 'source_test.bin'
    REBUILT_FILE = 'rebuilt_test.bin'
    KEY = 'integrity_test_file'

    print(f"Generating {FILE_SIZE} bytes of random data...")
    source_data = os.urandom(FILE_SIZE)
    with open(SOURCE_FILE, 'wb') as f:
        f.write(source_data)
    
    source_hash = hashlib.sha256(source_data).hexdigest()
    print(f"Source SHA256: {source_hash}")

    client = WaddleClient(HOST, PORT)
    
    print(f"Inserting data into WaddleMap under key: {KEY}")
    # Clear key if possible? WaddleMap currently doesn't have a 'delete' or 'clear'.
    # We'll use a unique key for this test.
    import uuid
    unique_key = f"{KEY}_{uuid.uuid4().hex[:8]}"
    
    for i in range(0, FILE_SIZE, CHUNK_SIZE):
        chunk = source_data[i:i+CHUNK_SIZE]
        resp = client.add_value(unique_key, chunk)
        if not resp.success:
            print(f"Failed to add chunk at offset {i}: {resp.error_message}")
            return

    print("Retrieving data from WaddleMap...")
    resp = client.get_value_list(unique_key)
    if not resp.success:
        print(f"Failed to retrieve value list: {resp.error_message}")
        return
    
    rebuilt_data = b''
    for item in resp.value_list.items:
        rebuilt_data += item.payload

    with open(REBUILT_FILE, 'wb') as f:
        f.write(rebuilt_data)
    
    rebuilt_hash = hashlib.sha256(rebuilt_data).hexdigest()
    print(f"Rebuilt SHA256: {rebuilt_hash}")

    if source_hash == rebuilt_hash:
        print("\n[SUCCESS] Source and rebuilt data are IDENTICAL!")
        print(f"Size: {len(rebuilt_data)} bytes")
    else:
        print("\n[FAIL] Data mismatch!")
        print(f"Source size: {len(source_data)}")
        print(f"Rebuilt size: {len(rebuilt_data)}")
    
    client.close()

if __name__ == "__main__":
    main()
