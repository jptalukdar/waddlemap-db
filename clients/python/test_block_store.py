import unittest
import time
from waddle_client import WaddleClient


class TestBlockStore(unittest.TestCase):
    def setUp(self):
        self.client = WaddleClient()
        self.collection_name = "python_test_col"
        # Cleanup before test
        try:
            self.client.delete_collection(self.collection_name)
        except:
            pass
        self.collection = self.client.create_collection(self.collection_name, 4, "l2")

    def tearDown(self):
        self.collection.delete()
        self.client.close()

    def test_complete_cycle(self):
        # 1. Append Block
        key = "doc_py"
        primary = "Python Client Test"
        vector = [0.1, 0.2, 0.3, 0.4]
        keywords = ["python", "client"]

        print(f"Appending block to key '{key}'...")
        resp = self.collection.append_block(key, primary, vector, keywords)
        # Note: server returns WaddleResponse, client returns it.
        # Check success manually if wrapper doesn't raise exception.
        # Wrapper raises Exception on !success.

        # 2. Get Block
        print("Retrieving block...")
        block = self.collection.get_block(key, 0)
        self.assertEqual(block.primary, primary)
        self.assertEqual(len(block.vector), 4)
        self.assertAlmostEqual(block.vector[0], 0.1)
        self.assertEqual(block.keywords, keywords)

        # 3. Search
        print("Searching...")
        results = self.collection.search(vector, top_k=1)
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0].key, key)
        self.assertEqual(results[0].block.primary, primary)

        # 4. Keyword Search
        print("Keyword searching...")
        keys = self.collection.keyword_search(["python"])
        self.assertIn(key, keys)

        # 5. Delete Key
        print("Deleting key...")
        self.collection.delete_key(key)

        # 6. Verify Deletion
        print("Verifying deletion...")
        # GetBlock should likely fail or return empty/error?
        # Protocol: GetBlock logic in server calls Manager.Get. If deleted, index in Collection is gone, but Manager?
        # VectorManager.DeleteKey logs delete, removes from Collection indexes.
        # GetBlock calls Collection.GetBlockVectorID. If key deleted from Collection, it fails.
        with self.assertRaises(Exception):
            self.collection.get_block(key, 0)

        # Search should return 0 results
        results = self.collection.search(vector, top_k=1)
        self.assertEqual(len(results), 0)


if __name__ == "__main__":
    unittest.main()
