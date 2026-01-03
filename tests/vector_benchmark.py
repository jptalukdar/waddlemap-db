import time
import sys
import os
import random
import uuid

# Add clients/python to path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "clients", "python"))

from waddle_client import WaddleClient

def generate_random_vector(dim):
    return [random.random() for _ in range(dim)]

def run_benchmark():
    HOST = "localhost"
    PORT = 6969
    COLLECTION_NAME = "benchmark_vec"
    DIMENSIONS = 128
    NUM_ITEMS = 1000
    BATCH_SIZE = 100
    
    # 1. Setup
    print(f"--- WaddleDB Vector Store Benchmark ---")
    print(f"Items: {NUM_ITEMS}, Dimensions: {DIMENSIONS}, Batch Size: {BATCH_SIZE}")
    
    client = WaddleClient(HOST, PORT)
    
    # Cleanup
    try:
        client.delete_collection(COLLECTION_NAME)
    except:
        pass
        
    collection = client.create_collection(COLLECTION_NAME, DIMENSIONS, "l2")
    
    # Generate Data
    print("Generating data...")
    data = []
    for i in range(NUM_ITEMS):
        data.append({
            "key": f"vec_{i}",
            "primary": f"Data payload for item {i}" * 5, # Some payload
            "vector": generate_random_vector(DIMENSIONS),
            "keywords": [f"tag_{i % 10}", "benchmark"]
        })
        
    # 2. Benchmark Ingestion (Batch)
    print("\n[Ingestion Benchmark]")
    start_time = time.time()
    
    for i in range(0, len(data), BATCH_SIZE):
        batch = data[i:i+BATCH_SIZE]
        collection.batch_append_blocks(batch)
        
    end_time = time.time()
    total_time = end_time - start_time
    qps = NUM_ITEMS / total_time
    print(f"Batch Ingestion: {total_time:.4f}s ({qps:.2f} items/s)")
    
    # 3. Benchmark Search (Latency)
    print("\n[Search Latency Benchmark]")
    query_vec = generate_random_vector(DIMENSIONS)
    
    latencies = []
    num_queries = 100
    
    start_total = time.time()
    for _ in range(num_queries):
        t0 = time.time()
        collection.search(query_vec, top_k=10)
        t1 = time.time()
        latencies.append((t1 - t0) * 1000) # ms
    end_total = time.time()
    
    avg_lat = sum(latencies) / len(latencies)
    qps_search = num_queries / (end_total - start_total)
    
    print(f"Search Latency (Avg): {avg_lat:.2f} ms")
    print(f"Search QPS: {qps_search:.2f}")

    # 4. Benchmark Retrieval (GetBlock)
    print("\n[Retrieval Latency Benchmark]")
    latencies_get = []
    
    for _ in range(num_queries):
        idx = random.randint(0, NUM_ITEMS - 1)
        key = f"vec_{idx}"
        t0 = time.time()
        collection.get_block(key, 0)
        t1 = time.time()
        latencies_get.append((t1 - t0) * 1000)
        
    avg_lat_get = sum(latencies_get) / len(latencies_get)
    print(f"GetBlock Latency (Avg): {avg_lat_get:.2f} ms")
    
    # Cleanup
    client.delete_collection(COLLECTION_NAME)
    client.close()
    print("\nDone.")

if __name__ == "__main__":
    run_benchmark()
