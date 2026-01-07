package types

// ProtocolMethod defines the operation type.
type ProtocolMethod int

const (
	OpCreateCollection ProtocolMethod = iota
	OpDeleteCollection
	OpListCollections
	OpCompactCollection
	OpAppendBlock
	OpGetBlock
	OpGetVector
	OpGetKeyLength
	OpGetKey
	OpDeleteKey
	OpListKeys
	OpContainsKey
	OpUpdateBlock
	OpReplaceBlock
	OpSearch
	OpSearchMLT
	OpSearchInKey
	OpKeywordSearch
	OpSnapshotCollection
	OpBatchAppendBlock
	OpGetRelativeBlocks
)

func (m ProtocolMethod) String() string {
	switch m {
	case OpCreateCollection:
		return "OpCreateCollection"
	case OpDeleteCollection:
		return "OpDeleteCollection"
	case OpListCollections:
		return "OpListCollections"
	case OpCompactCollection:
		return "OpCompactCollection"
	case OpAppendBlock:
		return "OpAppendBlock"
	case OpGetBlock:
		return "OpGetBlock"
	case OpGetVector:
		return "OpGetVector"
	case OpGetKeyLength:
		return "OpGetKeyLength"
	case OpGetKey:
		return "OpGetKey"
	case OpDeleteKey:
		return "OpDeleteKey"
	case OpListKeys:
		return "OpListKeys"
	case OpContainsKey:
		return "OpContainsKey"
	case OpUpdateBlock:
		return "OpUpdateBlock"
	case OpReplaceBlock:
		return "OpReplaceBlock"
	case OpSearch:
		return "OpSearch"
	case OpSearchMLT:
		return "OpSearchMLT"
	case OpSearchInKey:
		return "OpSearchInKey"
	case OpKeywordSearch:
		return "OpKeywordSearch"
	case OpSnapshotCollection:
		return "OpSnapshotCollection"
	case OpBatchAppendBlock:
		return "OpBatchAppendBlock"
	case OpGetRelativeBlocks:
		return "OpGetRelativeBlocks"
	default:
		return "Unknown"
	}
}

// DBSchemaConfig holds database configuration.
type DBSchemaConfig struct {
	PayloadSize int
	DataPath    string
	SyncMode    string // "strict" or "async"
}

// RequestContext carries request data through the pipeline.
type RequestContext struct {
	ReqID     string
	Operation ProtocolMethod
	Params    interface{}          // Wraps specific request struct
	RespChan  chan ResponseContext // Channel to send response back
}

// ResponseContext carries the result.
type ResponseContext struct {
	ReqID   string
	Success bool
	Data    interface{} // Resulting Item, Length, or Error message
	Error   error
}
