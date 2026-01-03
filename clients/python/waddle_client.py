import socket
import struct
import uuid
import os

import waddle_protocol_pb2 as pb


class Collection:
    """Represents a WaddleMap collection with all its operations."""

    def __init__(self, client, name):
        self.client = client
        self.name = name

    def append_block(self, key, primary, vector=None, keywords=None):
        """Append a block to a key in this collection."""
        req = pb.WaddleRequest()
        req.request_id = self.client._get_id()

        block = pb.BlockData()
        block.primary = primary
        if vector:
            block.vector.extend(vector)
        if keywords:
            block.keywords.extend(keywords)

        req.append_block.collection = self.name
        req.append_block.key = key
        req.append_block.block.CopyFrom(block)

        return self.client._send_request(req)

    def batch_append_blocks(self, items):
        """
        Batch append multiple blocks to this collection.

        Args:
            items: list of dicts with keys: 'key', 'primary', 'vector', 'keywords'
        """
        req = pb.WaddleRequest()
        req.request_id = self.client._get_id()

        req.batch_append.collection = self.name

        for item in items:
            append_req = req.batch_append.requests.add()
            append_req.collection = self.name
            append_req.key = item["key"]

            block = pb.BlockData()
            block.primary = item["primary"]
            if item.get("vector"):
                block.vector.extend(item["vector"])
            if item.get("keywords"):
                block.keywords.extend(item["keywords"])

            append_req.block.CopyFrom(block)

        return self.client._send_request(req)

    def get_block(self, key, index):
        """Get a specific block from a key in this collection."""
        req = pb.WaddleRequest()
        req.request_id = self.client._get_id()
        req.get_block.collection = self.name
        req.get_block.key = key
        req.get_block.index = index
        resp = self.client._send_request(req)
        return resp.block

    def delete_key(self, key):
        """Delete a key and all its blocks from this collection."""
        req = pb.WaddleRequest()
        req.request_id = self.client._get_id()
        req.delete_key.collection = self.name
        req.delete_key.key = key
        return self.client._send_request(req)

    def list_keys(self):
        """List all keys in this collection."""
        req = pb.WaddleRequest()
        req.request_id = self.client._get_id()
        req.list_keys.collection = self.name
        resp = self.client._send_request(req)
        return resp.key_list.keys

    def contains_key(self, key):
        """Check if a key exists in this collection."""
        req = pb.WaddleRequest()
        req.request_id = self.client._get_id()
        req.contains_key.collection = self.name
        req.contains_key.key = key
        resp = self.client._send_request(req)
        return resp.success

    def search(self, vector, top_k=10, keywords=None, mode="global"):
        """
        Perform vector search in this collection.

        Args:
            vector: Query vector
            top_k: Number of results to return
            keywords: Optional keyword filters
            mode: Search mode ("global" or "local")
        """
        req = pb.WaddleRequest()
        req.request_id = self.client._get_id()

        req.search.collection = self.name
        req.search.query.extend(vector)
        req.search.top_k = top_k
        req.search.mode = mode
        if keywords:
            req.search.keywords.extend(keywords)

        resp = self.client._send_request(req)
        return resp.search_list.results

    def keyword_search(self, keywords, mode="exact"):
        """
        Perform keyword search in this collection.

        Args:
            keywords: List of keywords to search for
            mode: Search mode ("exact" or other modes)
        """
        req = pb.WaddleRequest()
        req.request_id = self.client._get_id()

        req.keyword_search.collection = self.name
        req.keyword_search.keywords.extend(keywords)
        req.keyword_search.mode = mode

        resp = self.client._send_request(req)
        return resp.key_list.keys

    def delete(self):
        """Delete this entire collection."""
        req = pb.WaddleRequest()
        req.request_id = self.client._get_id()
        req.delete_col.name = self.name
        return self.client._send_request(req)


class WaddleClient:
    """Client for connecting to a WaddleMap database server."""

    def __init__(self, host="localhost", port=6969):
        self.host = host
        self.port = port
        self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.sock.connect((self.host, self.port))

    def close(self):
        """Close the connection to the server."""
        self.sock.close()

    def _get_id(self):
        return str(uuid.uuid4())

    def _send_request(self, req):
        data = req.SerializeToString()
        # Send Length (4 bytes big-endian) + Data
        length_prefix = struct.pack(">I", len(data))
        self.sock.sendall(length_prefix + data)

        # Read Length
        len_buf = self._recv_n(4)
        if not len_buf:
            raise Exception("Connection closed or empty response for length")
        msg_len = struct.unpack(">I", len_buf)[0]

        # Read Body
        resp_data = self._recv_n(msg_len)
        resp = pb.WaddleResponse()
        resp.ParseFromString(resp_data)

        if not resp.success:
            raise Exception(f"Server Error: {resp.error_message}")

        return resp

    def _recv_n(self, n):
        data = b""
        while len(data) < n:
            packet = self.sock.recv(n - len(data))
            if not packet:
                return None
            data += packet
        return data

    # --- Collection Management ---

    def create_collection(self, name, dimensions, metric="l2"):
        """
        Create a new collection and return a Collection object.

        Args:
            name: Collection name
            dimensions: Vector dimensions
            metric: Distance metric ("l2", "cosine", etc.)

        Returns:
            Collection object
        """
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.create_col.name = name
        req.create_col.dimensions = dimensions
        req.create_col.metric = metric
        self._send_request(req)
        return Collection(self, name)

    def collection(self, name):
        """
        Get a reference to an existing collection.

        Args:
            name: Collection name

        Returns:
            Collection object
        """
        return Collection(self, name)

    def delete_collection(self, name):
        """Delete a collection by name."""
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.delete_col.name = name
        return self._send_request(req)

    def list_collections(self):
        """List all collections in the database."""
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.list_cols.CopyFrom(pb.ListCollectionsRequest())
        resp = self._send_request(req)
        return resp.col_list.collections
