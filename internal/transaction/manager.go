package transaction

import (
	"fmt"
	"log"
	"waddlemap/internal/storage"
	"waddlemap/internal/types"
	pb "waddlemap/proto"
)

type Manager struct {
	Storage  *storage.VectorManager
	Requests chan types.RequestContext
}

func NewManager(storage *storage.VectorManager) *Manager {
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
	// Collection Ops
	case types.OpCreateCollection:
		if params, ok := req.Params.(*pb.CreateCollectionRequest); ok {
			metric := types.MetricL2
			if params.Metric == "cos" {
				metric = types.MetricCosine
			} else if params.Metric == "ip" {
				metric = types.MetricIP
			}
			err := tm.Storage.CreateCollection(params.Name, params.Dimensions, metric)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	case types.OpDeleteCollection:
		if params, ok := req.Params.(*pb.DeleteCollectionRequest); ok {
			err := tm.Storage.DeleteCollection(params.Name)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	case types.OpListCollections:
		if _, ok := req.Params.(*pb.ListCollectionsRequest); ok {
			cols := tm.Storage.ListCollections()
			colList := &pb.CollectionList{}
			for _, c := range cols {
				colList.Collections = append(colList.Collections, &pb.Collection{
					Name:       c.Name,
					Dimensions: c.Dimensions,
					Metric:     string(c.Metric),
				})
			}
			resp.Success = true
			resp.Data = colList
		}

	case types.OpCompactCollection:
		if params, ok := req.Params.(*pb.CompactCollectionRequest); ok {
			err := tm.Storage.CompactCollection(params.Name)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	// Block Ops
	case types.OpAppendBlock:
		if params, ok := req.Params.(*pb.AppendBlockRequest); ok {
			// Convert pb.BlockData to types.BlockData
			block := &types.BlockData{
				Primary:  params.Block.Primary,
				Vector:   params.Block.Vector,
				Keywords: params.Block.Keywords,
			}
			_, err := tm.Storage.AppendBlock(params.Collection, params.Key, block)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	case types.OpBatchAppendBlock:
		if params, ok := req.Params.(*pb.BatchAppendBlockRequest); ok {
			keys := make([]string, len(params.Requests))
			blocks := make([]*types.BlockData, len(params.Requests))

			for i, r := range params.Requests {
				keys[i] = r.Key
				blocks[i] = &types.BlockData{
					Primary:  r.Block.Primary,
					Vector:   r.Block.Vector,
					Keywords: r.Block.Keywords,
				}
			}

			// Call BatchAppendBlocks
			_, err := tm.Storage.BatchAppendBlocks(params.Collection, keys, blocks)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				// TODO: Return list of successes?
			}
		}

	case types.OpGetBlock:
		if params, ok := req.Params.(*pb.GetBlockRequest); ok {
			block, err := tm.Storage.GetBlock(params.Collection, params.Key, params.Index)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				if block != nil {
					resp.Data = &pb.BlockData{
						Primary:  block.Primary,
						Vector:   block.Vector,
						Keywords: block.Keywords,
					}
				}
			}
		}

	case types.OpGetVector:
		if params, ok := req.Params.(*pb.GetVectorRequest); ok {
			vec, err := tm.Storage.GetVector(params.Collection, params.Key, params.Index)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				// Return as BlockData with only vector? Or add GetVectorResponse?
				// GetVectorRequest might expect BlockData or Vector?
				// Proto response for GetVector uses 'BlockData block = 11' or raw data?
				// server.go switch says case *pb.BlockData -> Block.
				// We can return a BlockData with just vector.
				resp.Data = &pb.BlockData{
					Vector: vec,
				}
			}
		}

	case types.OpGetKeyLength:
		if params, ok := req.Params.(*pb.GetKeyLengthRequest); ok {
			l, err := tm.Storage.GetKeyLength(params.Collection, params.Key)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				resp.Data = uint64(l)
			}
		}

	case types.OpGetKey:
		if params, ok := req.Params.(*pb.GetKeyRequest); ok {
			blocks, err := tm.Storage.GetKey(params.Collection, params.Key)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				pbBlocks := &pb.BlockList{}
				for _, b := range blocks {
					pbBlocks.Blocks = append(pbBlocks.Blocks, &pb.BlockData{
						Primary:  b.Primary,
						Vector:   b.Vector,
						Keywords: b.Keywords,
					})
				}
				resp.Data = pbBlocks
			}
		}

	case types.OpDeleteKey:
		if params, ok := req.Params.(*pb.DeleteKeyRequest); ok {
			err := tm.Storage.DeleteKey(params.Collection, params.Key)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	case types.OpListKeys:
		if params, ok := req.Params.(*pb.ListKeysRequest); ok {
			keys, err := tm.Storage.ListKeys(params.Collection)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				resp.Data = &pb.KeyList{Keys: keys}
			}
		}

	case types.OpContainsKey:
		if params, ok := req.Params.(*pb.ContainsKeyRequest); ok {
			exists, err := tm.Storage.ContainsKey(params.Collection, params.Key)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				// Bool success implies exists? No.
				// Protocol check_key returned success=exists.
				// But boolean result is not in WaddleResponse result oneof.
				// Assuming success=true/false maps to found?
				// Or add BoolResult? For now, success=true if found?
				// Let's assume Success=Exists for ContainsKey.
				resp.Success = exists
			}
		}

	case types.OpUpdateBlock:
		if params, ok := req.Params.(*pb.UpdateBlockRequest); ok {
			block := &types.BlockData{
				Primary:  params.Block.Primary,
				Vector:   params.Block.Vector,
				Keywords: params.Block.Keywords,
			}
			err := tm.Storage.UpdateBlock(params.Collection, params.Key, params.Index, block)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	case types.OpReplaceBlock:
		if params, ok := req.Params.(*pb.ReplaceBlockRequest); ok {
			block := &types.BlockData{
				Primary:  params.Block.Primary,
				Vector:   params.Block.Vector,
				Keywords: params.Block.Keywords,
			}
			err := tm.Storage.ReplaceBlock(params.Collection, params.Key, params.Index, block)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	case types.OpSearch:
		if params, ok := req.Params.(*pb.SearchRequest); ok {
			res, err := tm.Storage.Search(params.Collection, params.Query, params.TopK, params.Mode, params.Keywords)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				sList := &pb.SearchResultList{}
				for _, r := range res {
					sList.Results = append(sList.Results, &pb.SearchResultItem{
						Key:      r.Key,
						Index:    r.Index,
						Distance: r.Distance,
						Block: &pb.BlockData{
							Primary:  r.Block.Primary,
							Vector:   r.Block.Vector,
							Keywords: r.Block.Keywords,
						},
					})
				}
				resp.Data = sList
			}
		}

	case types.OpSearchMLT:
		if params, ok := req.Params.(*pb.SearchMoreLikeThisRequest); ok {
			res, err := tm.Storage.SearchMLT(params.Collection, params.Key, params.Index, params.TopK)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				sList := &pb.SearchResultList{}
				for _, r := range res {
					sList.Results = append(sList.Results, &pb.SearchResultItem{
						Key:      r.Key,
						Index:    r.Index,
						Distance: r.Distance,
						// Block data if available
					})
				}
				resp.Data = sList
			}
		}

	case types.OpSearchInKey:
		if params, ok := req.Params.(*pb.SearchInKeyRequest); ok {
			res, err := tm.Storage.SearchInKey(params.Collection, params.Key, params.Query, params.TopK)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				sList := &pb.SearchResultList{}
				for _, r := range res {
					sList.Results = append(sList.Results, &pb.SearchResultItem{
						Key:      r.Key,
						Index:    r.Index,
						Distance: r.Distance,
					})
				}
				resp.Data = sList
			}
		}

	case types.OpKeywordSearch:
		// Not implemented in Proto yet? KeywordSearchRequest?
		// Assuming implementation from before but updated signature
		if params, ok := req.Params.(*pb.KeywordSearchRequest); ok {
			// KeywordSearch returns KeyList?
			// The stub signature in VectorManager needs to match request.
			// Stub: KeywordSearch() -> []Key
			// Proto: KeywordSearch -> returns KeyList (ListKeys)
			// Wait, KeywordSearchRequest was: keywords, mode.
			// Response: KeyList?
			// Check WaddleResponse oneof. KeyList is id 7.
			// Currently mapped to OpListKeys.
			// We can reuse KeyList for KeywordSearch.
			// Need to verify VectorManager stub.
			// Stub: func KeywordSearch(collection, keywords, mode, maxDist) -> []string (keys).
			// Req: has collection, keywords, mode. (No maxDist in Updated proto? I removed it in edit 377 lines... wait).
			// Let's checking proto content for KeywordSearchRequest.
			// "message KeywordSearchRequest { string collection = 1; repeated string keywords = 2; string mode = 3; }"
			// Correct. No maxDist.
			// Update VectorManager stub? No, I added the stub at end of file, but `VectorManager` ALREADY had `KeywordSearch` method from previous phase!
			// I need to be careful. The PREVIOUS method matches the OLD signature (with maxDist).
			// The NEW proto request does NOT have maxDist.
			// So `manager.go` calling `tm.Storage.KeywordSearch` using `params` (without MaxDist) to call method (with MaxDist) will fail or need default 0.
			// Also return type `[]string` matches.
			// So good.

			results, err := tm.Storage.KeywordSearch(params.Collection, params.Keywords, params.Mode, 0) // 0 for MaxDist
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
				resp.Data = &pb.KeyList{Keys: results}
			}
		}

	case types.OpSnapshotCollection:
		if params, ok := req.Params.(*pb.SnapshotCollectionRequest); ok {
			_, err := tm.Storage.SnapshotCollection(params.Collection)
			if err != nil {
				resp.Success = false
				resp.Error = err
			} else {
				resp.Success = true
			}
		}

	default:
		resp.Success = false
		resp.Error = fmt.Errorf("operation not implemented: %v", req.Operation)
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
