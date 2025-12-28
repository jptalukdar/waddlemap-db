import socket
import struct
import waddle_protocol_pb2 as pb
import uuid

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
        # Send Length + Data
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
        return resp

    def _recv_n(self, n):
        data = b''
        while len(data) < n:
            packet = self.sock.recv(n - len(data))
            if not packet:
                return None
            data += packet
        return data

    # 1. Add Value
    def add_value(self, key, payload_bytes):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        
        item = pb.DataItem()
        item.payload = payload_bytes
        
        add_req = pb.AddValueRequest()
        add_req.key = key
        add_req.item.CopyFrom(item)
        
        req.add_val.CopyFrom(add_req)
        return self._send_request(req)

    # 2. Check Key
    def check_key(self, key):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        check = pb.CheckKeyRequest(key=key)
        req.check_key.CopyFrom(check)
        return self._send_request(req)

    # 3. Get Value
    def get_value(self, key, index):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        get_req = pb.GetValueByIndexRequest(key=key, index=index)
        req.get_val.CopyFrom(get_req)
        return self._send_request(req)
    
    # 4. Get Length
    def get_length(self, key):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        len_req = pb.GetLengthRequest(key=key)
        req.get_len.CopyFrom(len_req)
        return self._send_request(req)

    # 5. Update Value
    def update_value(self, key, index, payload_bytes):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        
        item = pb.DataItem()
        item.payload = payload_bytes
        
        upd_req = pb.UpdateValueRequest(key=key, index=index)
        upd_req.item.CopyFrom(item)
        
        req.update_val.CopyFrom(upd_req)
        return self._send_request(req)

    # 6. Search Global
    def search_global(self, pattern_bytes):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        
        search_req = pb.SearchGlobalRequest(pattern=pattern_bytes)
        req.search_global.CopyFrom(search_req)
        return self._send_request(req)

    # 7. Snapshot
    def snapshot(self, name):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        
        snap_req = pb.SnapshotRequest(snapshot_name=name)
        req.snapshot.CopyFrom(snap_req)
        return self._send_request(req)

    # 8. Get Keys
    def get_keys(self):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.get_keys.CopyFrom(pb.GetKeysRequest())
        return self._send_request(req)

    # 9. Get Value List
    def get_value_list(self, key):
        req = pb.WaddleRequest()
        req.request_id = self._get_id()
        req.get_val_list.CopyFrom(pb.GetValueListRequest(key=key))
        return self._send_request(req)
