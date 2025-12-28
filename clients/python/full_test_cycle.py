import os
import hashlib
import time
import subprocess
import shutil
from waddle_client import WaddleClient

def main():
    HOST = 'localhost'
    PORT = 6969
    FILE_SIZE = 512 * 1024 # 512 KB
    CHUNK_SIZE = 8 * 1024
    SOURCE_FILE = 'source_cycle.bin'
    KEY = 'cycle_test_file'

    print("DANGER: Deleting ALL data for a clean start...")
    subprocess.run(["powershell", "-Command", "Stop-Process -Name waddle-server -ErrorAction SilentlyContinue"], check=False)
    time.sleep(2)
    
    root_dir = os.path.abspath(os.path.join(os.getcwd(), "..", ".."))
    db_dir = os.path.join(root_dir, "waddlemap_db")
    server_exe = os.path.join(root_dir, "waddle-server.exe")

    if os.path.exists(db_dir):
        shutil.rmtree(db_dir)
    
    print("Starting server for initial load...")
    subprocess.Popen([server_exe], creationflags=subprocess.CREATE_NEW_CONSOLE, cwd=root_dir)
    time.sleep(3)

    print(f"Generating {FILE_SIZE} bytes of random data...")
    source_data = os.urandom(FILE_SIZE)
    with open(SOURCE_FILE, 'wb') as f:
        f.write(source_data)
    source_hash = hashlib.sha256(source_data).hexdigest()

    client = WaddleClient(HOST, PORT)
    print(f"Inserting data into WaddleMap under key: {KEY}")
    for i in range(0, FILE_SIZE, CHUNK_SIZE):
        chunk = source_data[i:i+CHUNK_SIZE]
        client.add_value(KEY, chunk)
    client.close()
    time.sleep(1)

    print("Shutting down server...")
    subprocess.run(["powershell", "-Command", "Stop-Process -Name waddle-server -ErrorAction SilentlyContinue"], check=False)
    time.sleep(2)

    print("Deleting index files to force REBUILD...")
    for f in os.listdir(db_dir):
        if f.endswith(".idx"):
            os.remove(os.path.join(db_dir, f))
            print(f"  Removed {f}")

    print("Restarting server...")
    subprocess.Popen([server_exe], creationflags=subprocess.CREATE_NEW_CONSOLE, cwd=root_dir)
    time.sleep(5)

    print("Retrieving data after REBUILD...")
    client = WaddleClient(HOST, PORT)
    resp = client.get_value_list(KEY)
    if not resp.success:
        print(f"Failed to retrieve: {resp.error_message}")
        client.close()
        return

    rebuilt_data = b''
    for item in resp.value_list.items:
        rebuilt_data += item.payload
    
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
