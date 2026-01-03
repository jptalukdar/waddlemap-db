import os
import hashlib
import time
import subprocess
import shutil
from waddle_client import WaddleClient


def main():
    HOST = "localhost"
    PORT = 6969
    FILE_SIZE = 512 * 1024  # 512 KB
    CHUNK_SIZE = 8 * 1024
    SOURCE_FILE = "source_cycle.bin"
    COLLECTION_NAME = "cycle_test"

    print("DANGER: Deleting ALL data for a clean start...")
    subprocess.run(
        [
            "powershell",
            "-Command",
            "Stop-Process -Name server -ErrorAction SilentlyContinue",
        ],
        check=False,
    )
    time.sleep(2)

    root_dir = os.path.abspath(os.path.join(os.getcwd(), "..", ".."))
    db_dir = os.path.join(root_dir, "waddlemap_db")
    server_exe = os.path.join(root_dir, "server.exe")

    if os.path.exists(db_dir):
        shutil.rmtree(db_dir)

    print("Starting server for initial load...")
    subprocess.Popen(
        [server_exe], creationflags=subprocess.CREATE_NEW_CONSOLE, cwd=root_dir
    )
    time.sleep(3)

    print(f"Generating {FILE_SIZE} bytes of random data...")
    source_data = os.urandom(FILE_SIZE)
    with open(SOURCE_FILE, "wb") as f:
        f.write(source_data)
    source_hash = hashlib.sha256(source_data).hexdigest()

    client = WaddleClient(HOST, PORT)

    # Create collection
    try:
        client.delete_collection(COLLECTION_NAME)
    except:
        pass
    collection = client.create_collection(COLLECTION_NAME, dimensions=0)

    print(f"Inserting data into WaddleMap...")
    import uuid

    unique_key = f"cycle_test_{uuid.uuid4().hex[:8]}"

    for i in range(0, FILE_SIZE, CHUNK_SIZE):
        chunk = source_data[i : i + CHUNK_SIZE]
        collection.append_block(unique_key, chunk.decode("latin-1"))

    client.close()
    time.sleep(1)

    print("Shutting down server...")
    subprocess.run(
        [
            "powershell",
            "-Command",
            "Stop-Process -Name server -ErrorAction SilentlyContinue",
        ],
        check=False,
    )
    time.sleep(2)

    print("Deleting index files to force REBUILD...")
    for f in os.listdir(db_dir):
        if f.endswith(".idx"):
            os.remove(os.path.join(db_dir, f))
            print(f"  Removed {f}")

    print("Restarting server...")
    subprocess.Popen(
        [server_exe], creationflags=subprocess.CREATE_NEW_CONSOLE, cwd=root_dir
    )
    time.sleep(5)

    print("Retrieving data after REBUILD...")
    client = WaddleClient(HOST, PORT)
    collection = client.collection(COLLECTION_NAME)

    rebuilt_data = b""
    index = 0
    while True:
        try:
            block = collection.get_block(unique_key, index)
            rebuilt_data += block.primary.encode("latin-1")
            index += 1
        except:
            break

    rebuilt_hash = hashlib.sha256(rebuilt_data).hexdigest()
    print(f"Source SHA256:  {source_hash}")
    print(f"Rebuilt SHA256: {rebuilt_hash}")

    if source_hash == rebuilt_hash:
        print("\n[SUCCESS] Data integrity preserved after index REBUILD!")
    else:
        print("\n[FAIL] Data corruption or incomplete retrieval!")
        print(f"Source size: {len(source_data)}")
        print(f"Rebuilt size: {len(rebuilt_data)}")

    client.close()


if __name__ == "__main__":
    main()
