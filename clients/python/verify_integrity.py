import os
import hashlib
from waddle_client import WaddleClient


def main():
    HOST = "localhost"
    PORT = 6969
    FILE_SIZE = 1 * 1024 * 1024  # 1 MB
    CHUNK_SIZE = 16 * 1024  # 16 KB
    SOURCE_FILE = "source_test.bin"
    REBUILT_FILE = "rebuilt_test.bin"
    COLLECTION_NAME = "integrity_test"

    print(f"Generating {FILE_SIZE} bytes of random data...")
    source_data = os.urandom(FILE_SIZE)
    with open(SOURCE_FILE, "wb") as f:
        f.write(source_data)

    source_hash = hashlib.sha256(source_data).hexdigest()
    print(f"Source SHA256: {source_hash}")

    client = WaddleClient(HOST, PORT)

    # Setup collection
    try:
        client.delete_collection(COLLECTION_NAME)
    except:
        pass

    collection = client.create_collection(COLLECTION_NAME, dimensions=0)

    print(f"Inserting data into WaddleMap under unique key")
    # Use a unique key for this test
    import uuid

    unique_key = f"integrity_test_{uuid.uuid4().hex[:8]}"

    for i in range(0, FILE_SIZE, CHUNK_SIZE):
        chunk = source_data[i : i + CHUNK_SIZE]
        try:
            # Store binary data as latin-1 encoded string
            collection.append_block(unique_key, chunk.decode("latin-1"))
        except Exception as e:
            print(f"Failed to add chunk at offset {i}: {e}")
            return

    print("Retrieving data from WaddleMap...")
    # Note: The current API doesn't have get_value_list equivalent
    # We need to iterate through blocks manually
    rebuilt_data = b""
    index = 0
    while True:
        try:
            block = collection.get_block(unique_key, index)
            rebuilt_data += block.primary.encode("latin-1")
            index += 1
        except:
            break  # No more blocks

    with open(REBUILT_FILE, "wb") as f:
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
