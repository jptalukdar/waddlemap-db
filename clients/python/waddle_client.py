import socket
import struct
import uuid
import os

import waddle_protocol_pb2 as pb

class WaddleClient:
    def __init__(self, host='localhost', port=6969):
        self.host = host
        self.port = port
        self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.sock.connect((self.host, self.port))

    def close(self):
        self.sock.close()

    def _get_id(self):
        return str(uuid.uuid4())

    def _send_request(self, req):
        data = req.SerializeToString()
        # Send Length (4 bytes big-endian) + Data
        length_prefix = struct.pack('>I', len(data))
        self.sock.sendall(length_prefix + data)
        
        # Read Length
        len_buf = self._recv_n(4)
        if not len_buf:
            raise Exception("Connection closed or empty response for length")
        msg_len = struct.unpack('>I', len_buf)[0]
        
        # Read Body
        resp_data = self._recv_n(msg_len)
        resp = pb.WaddleResponse()
        resp.ParseFromString(resp_data)
        
        if not resp.success:
            raise Exception(f"Server Error: {resp.error_message}")
            
        return resp

    def _recv_n(self, n):
        data = b''
        while len(data) < n:
            packet = self.sock.recv(n - len(data))
            if not packet:
                return None
            data += packet
        return data

    # --- Collection Ops ---

    def create_collection(self, name, dimensions, metric="l2"):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.create_col.name = name
        req.create_col.dimensions = dimensions
        req.create_col.metric = metric
        return self._send_request(req)

    def delete_collection(self, name):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.delete_col.name = name
        return self._send_request(req)

    def list_collections(self):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.list_cols.CopyFrom(pb.ListCollectionsRequest())
        resp = self._send_request(req)
        return resp.col_list.collections

    # --- Block Ops ---

    def append_block(self, collection, key, primary, vector=None, keywords=None):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        
        block = pb.BlockData()
        block.primary = primary
        if vector:
            block.vector.extend(vector)
        if keywords:
            block.keywords.extend(keywords)
            
        req.append_block.collection = collection
        req.append_block.key = key
        req.append_block.block.CopyFrom(block)
        
        return self._send_request(req)

    def get_block(self, collection, key, index):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.get_block.collection = collection
        req.get_block.key = key
        req.get_block.index = index
        resp = self._send_request(req)
        return resp.block

    def delete_key(self, collection, key):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.delete_key.collection = collection
        req.delete_key.key = key
        return self._send_request(req)
    
    def list_keys(self, collection):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.list_keys.collection = collection
        resp = self._send_request(req)
        return resp.key_list.keys

    def contains_key(self, collection, key):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.contains_key.collection = collection
        req.contains_key.key = key
        resp = self._send_request(req)
        return resp.success

    # --- Search Ops ---

    def search(self, collection, vector, top_k=10, keywords=None, mode="global"):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        
        req.search.collection = collection
        req.search.query.extend(vector)
        req.search.top_k = top_k
        req.search.mode = mode
        if keywords:
            req.search.keywords.extend(keywords)
            
        resp = self._send_request(req)
        return resp.search_list.results

    def keyword_search(self, collection, keywords, mode="exact"):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        
        req.keyword_search.collection = collection
        req.keyword_search.keywords.extend(keywords)
        req.keyword_search.mode = mode
        
        resp = self._send_request(req)
        return resp.key_list.keys # Keyword search returns keys?
