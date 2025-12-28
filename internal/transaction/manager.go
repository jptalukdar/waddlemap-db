package transaction

import (
	"fmt"
	"log"
	"waddlemap/internal/storage"
	"waddlemap/internal/types"
	pb "waddlemap/proto"
)

type Manager struct {
	Storage  *storage.Manager
	Requests chan types.RequestContext
}

func NewManager(storage *storage.Manager) *Manager {
	return &Manager{
		Storage:  storage,
		Requests: make(chan types.RequestContext, 100),
	}
}

func (tm *Manager) Start() {
	go tm.dispatch()
}

func (tm *Manager) dispatch() {
	for req := range tm.Requests {
		go tm.handle(req)
	}
}

func (tm *Manager) handle(req types.RequestContext) {
	var resp types.ResponseContext
	resp.ReqID = req.ReqID
	log.Printf("Transaction Manager: Handling request %s (op: %d)\n", req.ReqID, req.Operation)
	switch req.Operation {

	// 1. AddValue
	case types.OpAddValue:
		if params, ok := req.Params.(*pb.AddValueRequest); ok {
			log.Printf("  Key: %s, Payload Size: %d\n", params.Key, len(params.Item.Payload))
			if err := tm.Storage.Append(params.Key, params.Item.Payload); err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		} else {
			resp.Error = fmt.Errorf("invalid params")
		}

	// 2. Check Key
	case types.OpCheckKey:
		if params, ok := req.Params.(*pb.CheckKeyRequest); ok {
			len := tm.Storage.GetLength(params.Key)
			resp.Success = len > 0
		} else {
			resp.Error = fmt.Errorf("invalid params")
		}

	// 3. Get Value
	case types.OpGetValue:
		if params, ok := req.Params.(*pb.GetValueByIndexRequest); ok {
			payload, err := tm.Storage.Get(params.Key, int(params.Index))
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				// Respond with Item
				item := &pb.DataItem{
					Id:      0, // ID system not fully impl yet
					Payload: payload,
				}
				resp.Data = item
			}
		}

	// 4. Get Length
	case types.OpGetLength:
		if params, ok := req.Params.(*pb.GetLengthRequest); ok {
			resp.Success = true
			resp.Data = uint64(tm.Storage.GetLength(params.Key))
		}

	// 5. Update Value
	case types.OpUpdateValue:
		if params, ok := req.Params.(*pb.UpdateValueRequest); ok {
			if err := tm.Storage.Update(params.Key, int(params.Index), params.Item.Payload); err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	// 6. Search Global
	case types.OpSearchGlobal:
		if params, ok := req.Params.(*pb.SearchGlobalRequest); ok {
			results, err := tm.Storage.SearchGlobal(params.Pattern)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				// Pack results
				searchRes := &pb.SearchResult{}
				for _, r := range results {
					searchRes.Items = append(searchRes.Items, &pb.DataItem{Payload: r})
				}
				resp.Data = searchRes
			}
		}

	// 7. Snapshot
	case types.OpSnapshot:
		if params, ok := req.Params.(*pb.SnapshotRequest); ok {
			if err := tm.Storage.Snapshot(params.SnapshotName); err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	// 8. Get Keys
	case types.OpGetKeys:
		if _, ok := req.Params.(*pb.GetKeysRequest); ok {
			keys := tm.Storage.GetKeys()
			resp.Success = true
			resp.Data = &pb.KeyList{Keys: keys}
		}

	// 9. Get Value List
	case types.OpGetValueList:
		if params, ok := req.Params.(*pb.GetValueListRequest); ok {
			log.Printf("  Key: %s\n", params.Key)
			payloads, err := tm.Storage.GetAllValues(params.Key)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				// Pack
				valList := &pb.ValueList{}
				for _, p := range payloads {
					valList.Items = append(valList.Items, &pb.DataItem{Payload: p})
				}
				resp.Data = valList
			}
		}

	default:
		resp.Success = false
		resp.Error = fmt.Errorf("operation not implemented")
	}

	// Error safety
	if resp.Error != nil {
		resp.Success = false
	}

	select {
	case req.RespChan <- resp:
	default:
	}
}
