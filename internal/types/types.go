package types

// ProtocolMethod defines the operation type.
type ProtocolMethod int

const (
	OpCheckKey ProtocolMethod = iota
	OpGetValue
	OpGetLength
	OpGetLast
	OpAddValue
	OpUpdateValue
	OpSearchGlobal
	OpSearchKey
	OpSnapshot
	OpGetKeys
	OpGetValueList
)

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
