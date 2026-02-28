# dag-go v1.0.0 Release Notes

**Release date:** 2026-02-27
**Go module:** `github.com/seoyhaein/dag-go`
**Minimum Go version:** 1.22

---

## Overview

v1.0.0 marks the first stable release of **dag-go** — a pure-Go, zero-dependency
(no Kubernetes, no external orchestration framework) Directed Acyclic Graph (DAG)
execution library.

The library provides a concurrency-safe, context-aware execution engine for
pipeline-style workloads: data pipelines, build systems, workflow automation,
and any task graph that must respect dependency ordering while running independent
steps in parallel.

---

## What's New (since development baseline)

### Core Library

| Area | Change |
|---|---|
| `Runnable` interface | `RunE(ctx context.Context, a interface{}) error` — context propagation throughout |
| `SafeChannel[T]` | Generic, double-close-safe channel wrapper for all inter-node communication |
| `DagConfig` | Configurable channel buffers, worker pool size, timeouts, error drain timeout |
| `DagWorkerPool` | `sync.Once`-guarded `Close()` eliminates double-close panic |
| `TransitionStatus` | CAS-guarded node state machine — illegal transitions are blocked atomically |
| `fanIn` | Refactored to `errgroup.WithContext`; cancellation propagates correctly |
| `detectCycle` / `DetectCycle` | Lock-split: internal (no lock) vs exported (RLock); `FinishDag` re-entrancy fixed |
| `ErrCycleDetected` | Sentinel error; `errors.Is`-compatible |
| `ErrNoRunner` | Sentinel error; returned when a node has no configured `Runnable` |
| `reportError` | Structured logrus logging: `dag_id` + `error` fields |
| `collectErrors` | Configurable drain timeout via `DagConfig.ErrorDrainTimeout` |

### New in v1.0.0

#### `Dag.Reset()` — Zero-cost DAG reuse
```go
// After Wait returns, Reset() restores the DAG to its initial execution state
// without rebuilding the graph.  Topology, runners, and configuration are preserved.
dag.Reset()
dag.ConnectRunner()
dag.GetReady(ctx)
dag.Start()
dag.Wait(ctx)
```

#### `Dag.ToMermaid()` — Built-in graph visualisation
```go
// Generates a Mermaid flowchart string from dag.Edges.
// Per-node runner types are embedded as node label hints.
fmt.Println(dag.ToMermaid())
```

Example output:
```
graph TD
    start_node(["start_node"])
    fetch["fetch\n*main.TransientRunner"]
    process["process\n*main.StableRunner"]
    save["save\n*main.StableRunner"]
    end_node(["end_node"])
    start_node --> fetch
    fetch --> process
    process --> save
    save --> end_node
```
Paste the output at <https://mermaid.live> to render the diagram.

#### `Dag.SetNodeRunners()` — Bulk runner assignment
```go
applied, missing, skipped := dag.SetNodeRunners(map[string]Runnable{
    "extract": &ExtractRunner{},
    "load":    &LoadRunner{},
})
```

#### Comprehensive godoc
All exported types, functions, and fields now carry detailed documentation
comments covering construction order, concurrency guarantees, and usage caveats.

---

## DAG Lifecycle

```
InitDag()
  └─ AddEdge / CreateNode + AddEdgeIfNodesExist
       └─ FinishDag()          ← seals graph; runs cycle detection
            └─ ConnectRunner() ← attaches 3-phase runner closures
                 └─ GetReady(ctx)  ← initialises worker pool
                      └─ Start()  ← fires trigger signal
                           └─ Wait(ctx)  ← blocks until completion or failure
                                └─ [Reset() → ConnectRunner → GetReady → Start → Wait]
```

---

## Node State Machine

```
Pending ──► Running ──► Succeeded
   │                └──► Failed
   └────────────────────► Skipped   (parent failed)
```

Transitions are enforced atomically via `TransitionStatus(from, to)`.
Terminal states (`Succeeded`, `Failed`, `Skipped`) have no outgoing transitions.

---

## Configuration Reference

```go
dag.Config = dag.DagConfig{
    MinChannelBuffer:  5,              // edge channel buffer
    MaxChannelBuffer:  100,            // NodesResult / Errors channel buffer
    StatusBuffer:      10,             // reserved for future use
    WorkerPoolSize:    50,             // max concurrent node goroutines
    DefaultTimeout:    30 * time.Second, // preFlight per-node timeout
    ErrorDrainTimeout: 5 * time.Second,  // collectErrors drain limit
}
```

---

## Runner Priority (highest → lowest)

1. Per-node override — `dag.SetNodeRunner(id, r)` / `SetNodeRunners(m)`
2. Dynamic resolver — `dag.SetRunnerResolver(rr)`
3. Global default — `dag.SetContainerCmd(r)`

---

## Breaking Changes

None — this is the first stable release.  The API surface defined in v1.0.0 is
the baseline for semantic versioning going forward.

---

## Dependency Policy

dag-go is intentionally dependency-light:

| Dependency | Purpose |
|---|---|
| `golang.org/x/sync` | `errgroup` for `fanIn` |
| `github.com/google/uuid` | DAG / node ID generation |
| `github.com/sirupsen/logrus` | structured logging |
| `github.com/seoyhaein/utils` | string utilities |

**Prohibited:** `k8s.io/*`, `sigs.k8s.io/*` — dag-go is a pure library with no
orchestration framework dependencies.

Test-only: `go.uber.org/goleak` for goroutine leak detection.

---

## Examples

Three runnable examples are included in the `examples/` directory:

| Example | Description |
|---|---|
| `examples/etl_pipeline` | Fan-out / fan-in ETL pipeline; shows error propagation when one source fails |
| `examples/build_system` | Multi-level build dependency graph; bulk runner assignment; Mermaid output |
| `examples/reset_retry` | Transient failure on first run; `Reset()` → successful retry |

```bash
go run ./examples/etl_pipeline/
go run ./examples/build_system/
go run ./examples/reset_retry/
```

---

## Testing

```bash
go test -race ./...      # unit + integration tests with race detector
go test -bench=. ./...   # benchmark suite
```

All tests pass with **0 data races detected** under `-race`.

---

## Known Limitations / Future Work

- `ErrorPolicy` enum (`FailFast` vs `ContinueOnError`) is planned for v1.1.0.
- `ErrNoRunner` carries no node-ID context yet; a `NodeError` struct is planned.
- `Dag.ErrCount()` helper (expose Errors channel depth without draining) is planned.
- Bounded semaphore for the `preFlight` errgroup is under evaluation.

---

## Credits

Developed and maintained by [@seoyhaein](https://github.com/seoyhaein).

API documentation: <https://pkg.go.dev/github.com/seoyhaein/dag-go>
