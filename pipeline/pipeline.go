package pipeline

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	dag "github.com/seoyhaein/dag-go"
)

type Pipeline struct {
	Id   string
	Dags []*dag.Dag
}

// 파이프라인은 dag 와 데이터를 연계해야 하는데 데이터의 경우는 다른 xml 처리하는 것이 바람직할 것이다.
// 외부에서 데이터를 가지고 올 경우, ftp 나 scp 나 기타 다른 프롤토콜을 사용할 경우도 생각을 해야 한다.

func NewPipeline() *Pipeline {

	return &Pipeline{
		Id: uuid.NewString(),
	}
}

// TODO 모든 dag 들을 실행 시킬 수 있어야 한다.

func (pipe *Pipeline) Start(ctx context.Context, dag *dag.Dag) {
	dag.DagSetFunc(ctx)
	dag.GetReady(ctx)
	dag.Start()
	dag.WaitTilOver(ctx)
}

func (pipe *Pipeline) Stop(ctx context.Context, dag *dag.Dag) {
	time.After(time.Second * 2)
}

func (pipe *Pipeline) ReStart(ctx context.Context, dag *dag.Dag) {

}

func (pipe *Pipeline) NewDags() *dag.Dag {
	n := 1
	pid := pipe.Id
	dags := len(pipe.Dags)

	if dags > 0 {
		n = dags + 1
	}
	sn := strconv.Itoa(n)
	dag := dag.NewDagWithPId(pid, sn)

	if dag == nil {
		return nil
	}

	pipe.Dags = append(pipe.Dags, dag)

	return dag
}

// find dag from pipeline 생각하기