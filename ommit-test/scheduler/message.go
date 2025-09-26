package scheduler

import "time"

// ===== Message & Event types =====
// 초기 구현, gRPC로 오갈 메시지 골격 proto-like POJOs
// 서버-스트리밍 RPC 방식.

// RunReq client -> server 메세지, TODO 수정될 수 있음.
type RunReq struct {
	RunID   string
	NodeID  string
	IdemKey string // idempotency key from executor
	Params  map[string]string
}

// RunRespEvent server -> client 메세지 TODO 수정될 수 있음.
type RunRespEvent struct {
	When   time.Time
	Level  string // INFO/WARN/ERROR
	State  string // Created/Starting/Running/Succeeded/Failed
	Msg    string
	RunID  string
	NodeID string
	// spawnId included for tracing
	SpawnID string
}

// NewRunReq 일단 이렇게 만들어놈. RunReq 값으로 처리했는데 불변성에 대한 요구도가 큰지 아니면 효율적인 측면이 큰지는 생각해보자.

func NewRunReq(runID, nodeID, idemKey string, params map[string]string) RunReq {
	if params == nil {
		params = make(map[string]string, 4)
	}
	return RunReq{
		RunID:   runID,
		NodeID:  nodeID,
		IdemKey: idemKey,
		Params:  params,
	}
}

/*func ReceiveFromExecutor(d *Dispatcher, req RunReq, sink EventSink) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return d.DispatchRun(ctx, &req, sink)
}*/
