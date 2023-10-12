package fleet

// Phase 1a:
type Prepare struct {
	PrepareId uint64 `json:"prepareId"` // proposal
	NodeId    string `json:"nodeId"`    // tie-breaker
	RoundNum  uint64 `json:"roundNum"`  // multipaxos round number
}

// Phase 1b:
type Promise struct {
	Error          string `json:"error"`     // non-nil = NACK
	PrepareId      uint64 `json:"prepareId"` // proposal
	NodeId         string `json:"nodeId"`    // tie-breaker
	RoundNum       uint64 `json:"roundNum"`  // multipaxos round number
	LastPrepareId  uint64 `json:"lastPrepareId"`
	LastAcceptedId uint64 `json:"lastAcceptedId"`
	Value          string `json:"value"`
}

// Phase 2a:
type Accept struct {
	AcceptId uint64 `json:"acceptId"`
	NodeId   string `json:"nodeId"` // tie-breaker
	Value    string `json:"value"`
}

// Phase 2b:
type Accepted struct {
	Error    string `json:"error"` // non-nil = NACK
	AcceptId uint64 `json:"acceptId"`
	NodeId   string `json:"nodeId"` // tie-breaker
}
