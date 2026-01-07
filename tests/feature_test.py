import sys
import os
import time
import uuid
import random

# Add clients/python to path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "clients", "python"))

from waddle_client import WaddleClient

class bcolors:
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'

class TestContext:
    def __init__(self, host="localhost", port=6969):
        self.client = WaddleClient(host, port)
        self.collections_created = []

    def cleanup(self):
        print(f"\n{bcolors.WARNING}Cleaning up created collections...{bcolors.ENDC}")
        for name in self.collections_created:
            try:
                self.client.delete_collection(name)
                print(f"  Deleted {name}")
            except Exception as e:
                print(f"  Failed to delete {name}: {e}")
        self.client.close()

    def create_collection(self, name, dims=128, metric="l2"):
        # Ensure clean state
        try:
            self.client.delete_collection(name)
        except:
            pass
            
        col = self.client.create_collection(name, dims, metric)
        if name not in self.collections_created:
            self.collections_created.append(name)
        return col

class BaseTest:
    def __init__(self, context):
        self.ctx = context

    def log(self, msg):
        print(f"    {msg}")

    def assert_true(self, condition, msg):
        if not condition:
            raise AssertionError(msg)
    
    def assert_equal(self, a, b, msg):
        if a != b:
            raise AssertionError(f"{msg}: {a} != {b}")

    def run(self):
        raise NotImplementedError

# --- Feature Tests ---

class CollectionManagementTest(BaseTest):
    def run(self):
        print(f"{bcolors.HEADER}[Test] Collection Management{bcolors.ENDC}")
        
        name = "test_col_mgmt"
        
        # 1. Create
        self.log(f"Creating collection '{name}'...")
        col = self.ctx.create_collection(name, 64, "cosine")
        self.assert_true(col is not None, "Collection object should be returned")
        
        # 2. List
        self.log("Listing collections...")
        cols = self.ctx.client.list_collections()
        found = False
        for c in cols:
            if c.name == name:
                found = True
                self.assert_equal(c.dimensions, 64, "Dimensions match")
                self.assert_equal(c.metric, "cosine", "Metric matches")
                break
        self.assert_true(found, f"Collection '{name}' found in list")
        
        # # 3. Delete
        # self.log(f"Deleting collection '{name}'...")
        # self.ctx.client.delete_collection(name)
        
        # # Verify delete
        # cols_after = self.ctx.client.list_collections()
        # found_after = any(c.name == name for c in cols_after)
        # self.assert_true(not found_after, f"Collection '{name}' should be deleted")
        
        # print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")

class BasicOperationsTest(BaseTest):
    def run(self):
        print(f"{bcolors.HEADER}[Test] Basic Operations (Append, Get, Delete){bcolors.ENDC}")
        name = "test_basic_ops"
        col = self.ctx.create_collection(name, 4, "l2")
        
        key = "record_1"
        data = "Hello World"
        vec = [0.1, 0.2, 0.3, 0.4]
        tags = ["test", "hello"]
        
        # 1. Append
        self.log(f"Appending block to '{key}'...")
        col.append_block(key, data, vector=vec, keywords=tags)
        
        # 2. Contains
        self.log("Checking contains_key...")
        exists = col.contains_key(key)
        self.assert_true(exists, "Key should exist")
        
        # 3. Get Block
        self.log("Getting block...")
        block = col.get_block(key, 0)
        self.assert_equal(block.primary, data, "Primary data match")
        # float comparison needs epsilon usually, but proto might float32 exact
        self.assert_true(len(block.vector) == 4, "Vector dim match")
        self.assert_equal(list(block.keywords), tags, "Keywords match")
        
        # 4. List Keys
        self.log("Listing keys...")
        keys = col.list_keys()
        self.assert_true(key in keys, "Key should be in list")
        
        # 5. Delete Key
        self.log("Deleting key...")
        col.delete_key(key)
        self.assert_true(not col.contains_key(key), "Key should be gone")
        
        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")

class BatchOperationsTest(BaseTest):
    def run(self):
        print(f"{bcolors.HEADER}[Test] Batch Operations{bcolors.ENDC}")
        name = "test_batch_ops"
        col = self.ctx.create_collection(name, 4, "l2")
        
        count = 50
        items = []
        for i in range(count):
            items.append({
                "key": f"batch_{i}",
                "primary": f"data_{i}",
                "vector": [random.random() for _ in range(4)],
                "keywords": [f"group_{i%5}"]
            })
            
        self.log(f"Batch appending {count} items...")
        start = time.time()
        col.batch_append_blocks(items)
        dur = time.time() - start
        self.log(f"Batch append took {dur:.4f}s")
        
        # Verify a random one
        idx = random.randint(0, count-1)
        k = items[idx]["key"]
        self.log(f"Verifying item '{k}'...")
        block = col.get_block(k, 0)
        self.assert_equal(block.primary, items[idx]["primary"], "Data match")
        
        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")

class SearchTest(BaseTest):
    def run(self):
        print(f"{bcolors.HEADER}[Test] Search Operations (Vector & Keyword){bcolors.ENDC}")
        name = "test_search"
        col = self.ctx.create_collection(name, 2, "l2")
        
        # Add 3 distinct points
        # A: [0, 0]
        # B: [1, 1]
        # C: [10, 10]
        points = [
            ("A", [0.0, 0.0], ["origin", "start"]),
            ("B", [1.0, 1.0], ["middle"]),
            ("C", [10.0, 10.0], ["far", "end"])
        ]
        
        for p in points:
            col.append_block(p[0], f"Data {p[0]}", vector=p[1], keywords=p[2])
            
        # 1. Vector Search (Nearest to [0.1, 0.1] should be A)
        self.log("Vector Search (Nearest to [0.1, 0.1])...")
        res = col.search([0.1, 0.1], top_k=1)
        self.assert_true(len(res) > 0, "Should get result")
        self.assert_equal(res[0].key, "A", "Nearest should be A")
        
        # 2. Keyword Search
        self.log("Keyword Search (tag 'far')...")
        keys = col.keyword_search(["far"])
        self.assert_true("C" in keys, "Should find C")
        self.assert_true("A" not in keys, "Should not find A")
        
        # 3. Search with Filter (Vector search near A, but filter for "middle")
        # Not fully supported in proto arguments for client.search yet? 
        # Client `search` has `keywords` arg.
        self.log("Hybrid Search (Near A, but tag 'middle')...")
        # Near A ([0,0]), but must have tag 'middle' (which is B [1,1])
        res = col.search([0.0, 0.0], top_k=1, keywords=["middle"])
        self.assert_true(len(res) > 0, "Should match B")
        self.assert_equal(res[0].key, "B", "Should filter to B")
        
        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")


class FileIntegrityTest(BaseTest):
    def run(self):
        print(f"{bcolors.HEADER}[Test] File Integrity (Chunking & Retrieval){bcolors.ENDC}")
        name = "test_integrity"
        # 1MB file, 64KB chunks
        total_size = 1024 * 1024
        chunk_size = 64 * 1024
        
        # Create collection
        # Dims 2, but we mostly care about primary storage here.
        col = self.ctx.create_collection(name, 2, "l2")
        
        # Generate random data
        self.log(f"Generating {total_size} bytes of random data...")
        import base64
        raw_data = os.urandom(total_size)
        # Encode to string because primary is string
        # Base64 expansion is ~33%. 1MB -> 1.33MB string.
        b64_data = base64.b64encode(raw_data).decode('utf-8')
        
        # Split into chunks
        chunks = []
        for i in range(0, len(b64_data), chunk_size):
            chunks.append(b64_data[i:i+chunk_size])
            
        key = "file_v1"
        self.log(f"Uploading {len(chunks)} chunks for key '{key}'...")
        
        start = time.time()
        for i, chunk in enumerate(chunks):
            # We can associate a vector or keyword if we want, but not strictly needed for storage test.
            # providing dummy vector
            col.append_block(key, chunk, vector=[0.0, 0.0], keywords=[f"chunk_{i}"])
            
        dur = time.time() - start
        self.log(f"Upload took {dur:.4f}s")
        
        self.log("Retrieving and reassembling...")
        retrieved_b64 = []
        for i in range(len(chunks)):
            block = col.get_block(key, i)
            retrieved_b64.append(block.primary)
            
        full_retrieved = "".join(retrieved_b64)
        
        self.log("Verifying integrity...")
        self.assert_equal(len(full_retrieved), len(b64_data), "Total length match")
        self.assert_equal(full_retrieved, b64_data, "Content match")
        
        # Double check decoding
        decoded = base64.b64decode(full_retrieved)
        self.assert_equal(decoded, raw_data, "Binary content match")
        
        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")

# --- Runner ---

def main():
    ctx = TestContext()
    
    tests = [
        CollectionManagementTest(ctx),
        BasicOperationsTest(ctx),
        BatchOperationsTest(ctx),
        SearchTest(ctx),
        FileIntegrityTest(ctx),
    ]
    
    print(f"Starting Feature Test Suite ({len(tests)} tests)...")
    print("-" * 50)
    
    success = True
    for t in tests:
        try:
            t.run()
        except AssertionError as e:
            print(f"{bcolors.FAIL}    FAIL: {e}{bcolors.ENDC}")
            success = False
        except Exception as e:
            print(f"{bcolors.FAIL}    ERROR: {e}{bcolors.ENDC}")
            import traceback
            traceback.print_exc()
            success = False
        print("-" * 50)
            
    ctx.cleanup()
    
    if success:
        print(f"{bcolors.OKGREEN}ALL TESTS PASSED{bcolors.ENDC}")
        sys.exit(0)
    else:
        print(f"{bcolors.FAIL}SOME TESTS FAILED{bcolors.ENDC}")
        sys.exit(1)

if __name__ == "__main__":
    main()
