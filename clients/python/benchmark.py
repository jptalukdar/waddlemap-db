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

def worker_write(pid, count):
    client = WaddleClient(HOST, PORT)
    payload = b'X' * PAYLOAD_SIZE
    start = time.time()
    
    for i in range(count):
        key = f"user_{pid}_{i}"
        resp = client.add_value(key, payload)
        if not resp.success:
            print(f"Write failed: {resp.error_message}")
            
    duration = time.time() - start
    client.close()
    return count, duration

def worker_read(pid, count):
    client = WaddleClient(HOST, PORT)
    start = time.time()
    
    for i in range(count):
        # Read a random key from own set
        idx = random.randint(0, count-1)
        key = f"user_{pid}_{idx}"
        # We know we only added 1 item per key in write phase, so index 0
        resp = client.get_value(key, 0)
        if not resp.success:
            # Might fail if we persist data but restart index (if not persisted properly)
            # But for this run, it should be fine.
            pass
            
    duration = time.time() - start
    client.close()
    return count, duration

def worker_update(pid, count):
    client = WaddleClient(HOST, PORT)
    payload = b'U' * PAYLOAD_SIZE
    start = time.time()
    
    for i in range(count):
        idx = random.randint(0, count-1)
        key = f"user_{pid}_{idx}"
        # Update index 0
        client.update_value(key, 0, payload)
            
    duration = time.time() - start
    client.close()
    return count, duration

def worker_check_key(pid, count):
    client = WaddleClient(HOST, PORT)
    start = time.time()
    
    for i in range(count):
        idx = random.randint(0, count-1)
        key = f"user_{pid}_{idx}"
        client.check_key(key)
            
    duration = time.time() - start
    client.close()
    return count, duration

def worker_get_length(pid, count):
    client = WaddleClient(HOST, PORT)
    start = time.time()
    
    for i in range(count):
        idx = random.randint(0, count-1)
        key = f"user_{pid}_{idx}"
        client.get_length(key)
            
    duration = time.time() - start
    client.close()
    return count, duration

def worker_get_value_list(pid, count):
    client = WaddleClient(HOST, PORT)
    start = time.time()
    
    for i in range(count):
        idx = random.randint(0, count-1)
        key = f"user_{pid}_{idx}"
        client.get_value_list(key)
            
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
    client = WaddleClient(HOST, PORT)
    start = time.time()
    resp = client.search_global(b'X' * 10) 
    duration = time.time() - start
    hits = len(resp.search_results.items) if resp.success else 0
    print(f"Search Time: {duration*1000:.2f} ms")
    print(f"Hits found: {hits}")

    # 8. Get Keys Phase (Single threaded latency test)
    print("\n--- Starting GET_KEYS Phase ---")
    start = time.time()
    resp = client.get_keys()
    duration = time.time() - start
    keys_count = len(resp.key_list.keys) if resp.success else 0
    print(f"GetKeys Time: {duration*1000:.2f} ms")
    print(f"Keys found: {keys_count}")

    client.close()

if __name__ == "__main__":
    main()
