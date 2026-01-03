import time
import random
import multiprocessing.dummy as multiprocessing
import sys
import os
from waddle_client import WaddleClient

# Configuration
HOST = 'localhost'
PORT = 6969
NUM_PROCESSES = 4
ITEMS_PER_PROCESS = 1000
PAYLOAD_SIZE = 128  # bytes
COLLECTION_NAME = "benchmark"

def worker_write(pid, count):
    client = WaddleClient(HOST, PORT)
    collection = client.collection(COLLECTION_NAME)
    payload = b'X' * PAYLOAD_SIZE
    start = time.time()
    
    for i in range(count):
        key = f"user_{pid}_{i}"
        try:
            collection.append_block(key, payload.decode('latin-1'))
        except Exception as e:
            print(f"Write failed: {e}")
            
    duration = time.time() - start
    client.close()
    return count, duration

def worker_read(pid, count):
    client = WaddleClient(HOST, PORT)
    collection = client.collection(COLLECTION_NAME)
    start = time.time()
    
    for i in range(count):
        # Read a random key from own set
        idx = random.randint(0, count-1)
        key = f"user_{pid}_{idx}"
        # We know we only added 1 item per key in write phase, so index 0
        try:
            collection.get_block(key, 0)
        except:
            # Might fail if we persist data but restart index (if not persisted properly)
            # But for this run, it should be fine.
            pass
            
    duration = time.time() - start
    client.close()
    return count, duration

def worker_check_key(pid, count):
    client = WaddleClient(HOST, PORT)
    collection = client.collection(COLLECTION_NAME)
    start = time.time()
    
    for i in range(count):
        idx = random.randint(0, count-1)
        key = f"user_{pid}_{idx}"
        try:
            collection.contains_key(key)
        except:
            pass
            
    duration = time.time() - start
    client.close()
    return count, duration

def worker_list_keys(pid, count):
    client = WaddleClient(HOST, PORT)
    collection = client.collection(COLLECTION_NAME)
    start = time.time()
    
    for i in range(count):
        try:
            collection.list_keys()
        except:
            pass
            
    duration = time.time() - start
    client.close()
    return count, duration

def run_phase(name, worker_func, total_items, concurrency):
    print(f"\n--- Starting {name} Phase ---")
    print(f"Items: {total_items}, Concurrency: {concurrency}")
    
    pool = multiprocessing.Pool(processes=concurrency)
    items_per_worker = total_items // concurrency
    
    results = []
    start_global = time.time()
    
    for i in range(concurrency):
        results.append(pool.apply_async(worker_func, (i, items_per_worker)))
        
    pool.close()
    pool.join()
    
    end_global = time.time()
    total_duration = end_global - start_global
    
    total_ops = 0
    for res in results:
        ops, _ = res.get()
        total_ops += ops
        
    ops_sec = total_ops / total_duration
    print(f"{name} Completed in {total_duration:.4f}s")
    print(f"Throughput: {ops_sec:.2f} OPS")
    return ops_sec

def main():
    print(f"Target: {HOST}:{PORT}")
    print(f"Payload: {PAYLOAD_SIZE} bytes")
    
    total_items = NUM_PROCESSES * ITEMS_PER_PROCESS
    
    # 1. Warmup / Write Phase
    run_phase("WRITE", worker_write, total_items, NUM_PROCESSES)
    
    # 2. Read Phase
    run_phase("READ", worker_read, total_items, NUM_PROCESSES)

    # 3. Update Phase
    run_phase("UPDATE", worker_update, total_items, NUM_PROCESSES)

    # 4. Check Key Phase
    run_phase("CHECK_KEY", worker_check_key, total_items, NUM_PROCESSES)

    # 5. Get Length Phase
    run_phase("GET_LENGTH", worker_get_length, total_items, NUM_PROCESSES)

    # 6. Get Value List Phase
    run_phase("GET_VAL_LIST", worker_get_value_list, total_items, NUM_PROCESSES)
    
    # 7. Search Phase (Single threaded latency test)
    print("\n--- Starting SEARCH Phase ---")
    # Initialize collection
    print("Setting up collection...")
    client = WaddleClient(HOST, PORT)
    try:
        client.delete_collection(COLLECTION_NAME)
    except:
        pass
    client.create_collection(COLLECTION_NAME, dimensions=0)  # No vectors for basic benchmark
    client.close()
    
    total_items = NUM_PROCESSES * ITEMS_PER_PROCESS
    
    # 1. Warmup / Write Phase
    run_phase("WRITE", worker_write, total_items, NUM_PROCESSES)
    
    # 2. Read Phase
    run_phase("READ", worker_read, total_items, NUM_PROCESSES)

    # 3. Check Key Phase
    run_phase("CHECK_KEY", worker_check_key, total_items, NUM_PROCESSES)

    # 4. List Keys Phase (reduced iterations as it's expensive)
    run_phase("LIST_KEYS", worker_list_keys, NUM_PROCESSES * 10, NUM_PROCESSES)
    
    # 5. Get Keys Phase (Single threaded latency test)
    print("\n--- Starting GET_KEYS Phase ---")
    client = WaddleClient(HOST, PORT)
    collection = client.collection(COLLECTION_NAME)
    start = time.time()
    try:
        keys = collection.list_keys()
        duration = time.time() - start
        keys_count = len(keys)
        print(f"GetKeys Time: {duration*1000:.2f} ms")
        print(f"Keys found: {keys_count}")
    except Exception as e:
        print(f"Error: {e