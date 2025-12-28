package network

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
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
	log.Printf("WaddleMap Server listening on port %d\n", s.Port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v\n", err)
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
				log.Printf("Read header error: %v\n", err)
			}
			return
		}
		msgLen := binary.BigEndian.Uint32(lenBuf)

		// 2. Read Message Body
		buf := make([]byte, msgLen)
		if _, err := io.ReadFull(conn, buf); err != nil {
			log.Printf("Read body error: %v\n", err)
			return
		}

		// Decode Protobuf
		var reqPb pb.WaddleRequest
		if err := proto.Unmarshal(buf, &reqPb); err != nil {
			log.Printf("Unmarshal error: %v\n", err)
			continue
		}

		// Map Proto Params to RequestContext
		ctx := types.RequestContext{
			ReqID:    reqPb.RequestId,
			RespChan: make(chan types.ResponseContext),
		}

		// Determine Operation
		switch op := reqPb.Operation.(type) {
		case *pb.WaddleRequest_AddVal:
			ctx.Operation = types.OpAddValue
			ctx.Params = op.AddVal
		case *pb.WaddleRequest_CheckKey:
			ctx.Operation = types.OpCheckKey
			ctx.Params = op.CheckKey
		case *pb.WaddleRequest_GetVal:
			ctx.Operation = types.OpGetValue
			ctx.Params = op.GetVal
		case *pb.WaddleRequest_GetLen:
			ctx.Operation = types.OpGetLength
			ctx.Params = op.GetLen
		case *pb.WaddleRequest_UpdateVal:
			ctx.Operation = types.OpUpdateValue
			ctx.Params = op.UpdateVal
		case *pb.WaddleRequest_SearchGlobal:
			ctx.Operation = types.OpSearchGlobal
			ctx.Params = op.SearchGlobal
		case *pb.WaddleRequest_Snapshot:
			ctx.Operation = types.OpSnapshot
			ctx.Params = op.Snapshot
		case *pb.WaddleRequest_GetKeys:
			ctx.Operation = types.OpGetKeys
			ctx.Params = op.GetKeys
		case *pb.WaddleRequest_GetValList:
			ctx.Operation = types.OpGetValueList
			ctx.Params = op.GetValList

		default:
			log.Printf("Unknown operation: %T\n", reqPb.Operation)
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
			log.Printf("Op Error (ReqID: %s): %v\n", respCtx.ReqID, respCtx.Error)
			respPb.ErrorMessage = respCtx.Error.Error()
		}

		// Map Result
		if respCtx.Data != nil {
			switch d := respCtx.Data.(type) {
			case *pb.DataItem:
				respPb.Result = &pb.WaddleResponse_Item{Item: d}
			case uint64:
				respPb.Result = &pb.WaddleResponse_Length{Length: d}
			case *pb.SearchResult:
				respPb.Result = &pb.WaddleResponse_SearchResults{SearchResults: d}
			case *pb.KeyList:
				respPb.Result = &pb.WaddleResponse_KeyList{KeyList: d}
			case *pb.ValueList:
				respPb.Result = &pb.WaddleResponse_ValueList{ValueList: d}
			}
		}

		data, err := proto.Marshal(respPb)
		if err != nil {
			log.Printf("Marshal error: %v\n", err)
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
