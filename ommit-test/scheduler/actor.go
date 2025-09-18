package scheduler

import "sync/atomic"

// --- Actor commands ---

type CommandKind uint8

const (
	CmdRun CommandKind = iota
	CmdCancel
)

type Command struct {
	Kind    CommandKind
	RunReq  RunReq
	SpawnID string
	Sink    EventSink
}

// ===== Actor =====

type Actor struct {
	spawnID string
	mbox    chan Command
	closing atomic.Bool
}

// ===== Event sink (simulating gRPC server stream) =====

type EventSink interface {
	Emit(ev RunRespEvent) error
}

type StdoutSink struct{}

// --- Driver (fake for demo) ---

type FakeDriver struct{}

func NewActor(spawnID string) *Actor {
	return &Actor{
		spawnID: spawnID,
		mbox:    make(chan Command, 64), // simple buffer
	}
}
