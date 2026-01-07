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

class RelativeBlockTest(BaseTest):
    def run(self):
        print(f"{bcolors.HEADER}[Test] Relative Block Operations (Next & Previous){bcolors.ENDC}")
        name = "test_relative_blocks"
        col = self.ctx.create_collection(name, 4, "l2")
        
        key = "sequence_key"
        # Create a sequence of 5 blocks
        # Block 0: "first", [0.0]*4
        # Block 1: "second", [0.1]*4
        # Block 2: "third", [0.2]*4
        # Block 3: "fourth", [0.3]*4
        # Block 4: "fifth", [0.4]*4
        
        blocks_data = ["first", "second", "third", "fourth", "fifth"]
        
        self.log(f"Appending {len(blocks_data)} blocks to '{key}'...")
        for i, data in enumerate(blocks_data):
            col.append_block(key, data, vector=[float(i)/10.0]*4)
            
        # 1. Test get_next_block
        self.log("Testing get_next_block...")
        
        # Next from 0 should be [current, next] -> [0, 1]
        next_blocks = col.get_next_block(key, 0)
        self.assert_equal(len(next_blocks), 2, "Should return 2 blocks (current, next)")
        self.assert_equal(next_blocks[0].primary, "first", "Index 0 is current")
        self.assert_equal(next_blocks[1].primary, "second", "Index 1 is next")
        
        # Next from 3 should be [3, 4]
        next_blocks_2 = col.get_next_block(key, 3)
        self.assert_equal(len(next_blocks_2), 2, "Should return 2 blocks")
        self.assert_equal(next_blocks_2[0].primary, "fourth", "Index 0 is current")
        self.assert_equal(next_blocks_2[1].primary, "fifth", "Index 1 is next")
        
        # Next from 4 (last) should be just [4] (current) as next is out of bounds
        next_blocks_3 = col.get_next_block(key, 4)
        self.assert_equal(len(next_blocks_3), 1, "Next from last should return only current")
        self.assert_equal(next_blocks_3[0].primary, "fifth", "Should be current block")

        # 2. Test get_previous_block
        self.log("Testing get_previous_block...")
        
        # Prev from 1 should be [prev, current] -> [0, 1]
        prev_blocks = col.get_previous_block(key, 1)
        self.assert_equal(len(prev_blocks), 2, "Should return 2 blocks (prev, current)")
        self.assert_equal(prev_blocks[0].primary, "first", "Index 0 is prev")
        self.assert_equal(prev_blocks[1].primary, "second", "Index 1 is current")
        
        # Prev from 4 should be [3, 4]
        prev_blocks_2 = col.get_previous_block(key, 4)
        self.assert_equal(len(prev_blocks_2), 2, "Should return 2 blocks")
        self.assert_equal(prev_blocks_2[0].primary, "fourth", "Index 0 is prev")
        self.assert_equal(prev_blocks_2[1].primary, "fifth", "Index 1 is current")
        
        # Prev from 0 (first) should be just [0] (current) as prev is out of bounds
        prev_blocks_3 = col.get_previous_block(key, 0)
        self.assert_equal(len(prev_blocks_3), 1, "Prev from first should return only current")
        self.assert_equal(prev_blocks_3[0].primary, "first", "Should be current block")

        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")

def main():
    ctx = TestContext()
    
    tests = [
        RelativeBlockTest(ctx),
    ]
    
    print(f"Starting Relative Block Test Suite ({len(tests)} tests)...")
    print("-" * 50)
    
    success = True
    for t in tests:
        try:
            t.run()
        except AssertionError as e:
            print(f"{bcolors.FAIL}    FAIL: {e}{bcolors.ENDC}")
            success = False
        except Exception as e:
            # print(f"{bcolors.FAIL}    ERROR: {e}{bcolors.ENDC}")
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
