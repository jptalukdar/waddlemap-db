package network

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"waddlemap/internal/logger"
	"waddlemap/internal/transaction"
	"waddlemap/internal/types"
	pb "waddlemap/proto"

	"google.golang.org/protobuf/proto"
)

type Server struct {
	Port      int
	TxManager *transaction.Manager
}

func NewServer(port int, txMgr *transaction.Manager) *Server {
	return &Server{
		Port:      port,
		TxManager: txMgr,
	}
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		return err
	}
	defer listener.Close()
	// logger.Info("WaddleMap Server listening on port %d", s.Port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			// logger.Error("Accept error: %v", err)
			continue
		}

		// Optimize Buffer Size
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetReadBuffer(65536) // 64KB
			tcpConn.SetWriteBuffer(65536)
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	for {
		// 1. Read Length Header (4 bytes)
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			if err != io.EOF {
				// logger.Error("Read header error: %v", err)
			}
			return
		}
		msgLen := binary.BigEndian.Uint32(lenBuf)

		// 2. Read Message Body
		buf := make([]byte, msgLen)
		if _, err := io.ReadFull(conn, buf); err != nil {
			// logger.Error("Read body error: %v", err)
			return
		}

		// Decode Protobuf
		var reqPb pb.WaddleRequest
		if err := proto.Unmarshal(buf, &reqPb); err != nil {
			// logger.Error("Unmarshal error: %v", err)
			continue
		}

		// Map Proto Params to RequestContext
		ctx := types.RequestContext{
			ReqID:    reqPb.RequestId,
			RespChan: make(chan types.ResponseContext),
		}

		// Determine Operation
		// Determine Operation
		switch op := reqPb.Operation.(type) {
		case *pb.WaddleRequest_CreateCol:
			ctx.Operation = types.OpCreateCollection
			ctx.Params = op.CreateCol
		case *pb.WaddleRequest_DeleteCol:
			ctx.Operation = types.OpDeleteCollection
			ctx.Params = op.DeleteCol
		case *pb.WaddleRequest_ListCols:
			ctx.Operation = types.OpListCollections
			ctx.Params = op.ListCols
		case *pb.WaddleRequest_CompactCol:
			ctx.Operation = types.OpCompactCollection
			ctx.Params = op.CompactCol
		case *pb.WaddleRequest_AppendBlock:
			ctx.Operation = types.OpAppendBlock
			ctx.Params = op.AppendBlock
		case *pb.WaddleRequest_GetBlock:
			ctx.Operation = types.OpGetBlock
			ctx.Params = op.GetBlock
		case *pb.WaddleRequest_GetVector:
			ctx.Operation = types.OpGetVector
			ctx.Params = op.GetVector
		case *pb.WaddleRequest_GetKeyLen:
			ctx.Operation = types.OpGetKeyLength
			ctx.Params = op.GetKeyLen
		case *pb.WaddleRequest_GetKey:
			ctx.Operation = types.OpGetKey
			ctx.Params = op.GetKey
		case *pb.WaddleRequest_DeleteKey:
			ctx.Operation = types.OpDeleteKey
			ctx.Params = op.DeleteKey
		case *pb.WaddleRequest_ListKeys:
			ctx.Operation = types.OpListKeys
			ctx.Params = op.ListKeys
		case *pb.WaddleRequest_ContainsKey:
			ctx.Operation = types.OpContainsKey
			ctx.Params = op.ContainsKey
		case *pb.WaddleRequest_UpdateBlock:
			ctx.Operation = types.OpUpdateBlock
			ctx.Params = op.UpdateBlock
		case *pb.WaddleRequest_ReplaceBlock:
			ctx.Operation = types.OpReplaceBlock
			ctx.Params = op.ReplaceBlock
		case *pb.WaddleRequest_Search:
			ctx.Operation = types.OpSearch
			ctx.Params = op.Search
		case *pb.WaddleRequest_SearchMlt:
			ctx.Operation = types.OpSearchMLT
			ctx.Params = op.SearchMlt
		case *pb.WaddleRequest_SearchInKey:
			ctx.Operation = types.OpSearchInKey
			ctx.Params = op.SearchInKey
		case *pb.WaddleRequest_KeywordSearch:
			ctx.Operation = types.OpKeywordSearch
			ctx.Params = op.KeywordSearch
		case *pb.WaddleRequest_SnapshotCol:
			ctx.Operation = types.OpSnapshotCollection
			ctx.Params = op.SnapshotCol
		case *pb.WaddleRequest_BatchAppend:
			ctx.Operation = types.OpBatchAppendBlock
			ctx.Params = op.BatchAppend
		case *pb.WaddleRequest_GetRelativeBlocks:
			ctx.Operation = types.OpGetRelativeBlocks
			ctx.Params = op.GetRelativeBlocks
		default:
			logger.Info("Unknown operation: %T", reqPb.Operation)
			continue
		}

		// Send to TxMgr
		s.TxManager.Requests <- ctx

		// Wait for Response
		respCtx := <-ctx.RespChan

		// Encode Response
		respPb := &pb.WaddleResponse{
			RequestId: respCtx.ReqID,
			Success:   respCtx.Success,
		}

		if respCtx.Error != nil {
			logger.Error("Op (%s) Error (ReqID: %s): %v", ctx.Operation.String(), respCtx.ReqID, respCtx.Error)
			respPb.ErrorMessage = respCtx.Error.Error()
		}

		// Map Result
		if respCtx.Data != nil {
			switch d := respCtx.Data.(type) {
			case uint64:
				respPb.Result = &pb.WaddleResponse_Length{Length: d}
			case *pb.KeyList:
				respPb.Result = &pb.WaddleResponse_KeyList{KeyList: d}
			case *pb.CollectionList:
				respPb.Result = &pb.WaddleResponse_ColList{ColList: d}
			case *pb.SearchResultList:
				respPb.Result = &pb.WaddleResponse_SearchList{SearchList: d}
			case *pb.BlockData:
				respPb.Result = &pb.WaddleResponse_Block{Block: d}
			case *pb.BlockList:
				respPb.Result = &pb.WaddleResponse_BlockList{BlockList: d}
			}
		}

		data, err := proto.Marshal(respPb)
		if err != nil {
			logger.Error("Marshal error: %v", err)
			return
		}

		// WRITE Response with Length Prefix
		respLenBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(respLenBuf, uint32(len(data)))

		if _, err := conn.Write(respLenBuf); err != nil {
			return
		}
		if _, err := conn.Write(data); err != nil {
			return
		}
	}
}
