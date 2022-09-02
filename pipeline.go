package dag_go

import (
	"context"
	"fmt"
	"github.com/seoyhaein/utils"
	"time"

	"github.com/google/uuid"
)

type Pipeline struct {
	Id   string
	Dags []*Dag

	ContainerCmd Runnable
}

// 파이프라인은 dag 와 데이터를 연계해야 하는데 데이터의 경우는 다른 xml 처리하는 것이 바람직할 것이다.
// 외부에서 데이터를 가지고 올 경우, ftp 나 scp 나 기타 다른 프롤토콜을 사용할 경우도 생각을 해야 한다.

// TODO Runnable 삭제 가능.
func NewPipeline() *Pipeline {

	return &Pipeline{
		Id: uuid.NewString(),
	}
}

// TODO 모든 dag 들을 실행 시킬 수 있어야 한다. 수정해줘야 한다
func (pipe *Pipeline) Start(ctx context.Context) {
	if pipe.Dags == nil {
		return
	}

	for _, d := range pipe.Dags {
		d.DagSetFunc()
		d.GetReady(ctx)
		d.Start()
		d.Wait(ctx)
	}
}

func (pipe *Pipeline) Stop(ctx context.Context, dag *Dag) {
	time.After(time.Second * 2)
}

func (pipe *Pipeline) ReStart(ctx context.Context, dag *Dag) {

}

// NewDags 파이프라인과 dag 의 차이점은 데이터의 차이이다.
// 즉, 같은 dag 이지만 데이터가 다를 수 있다.
// 파이프라인에서 데이터 연계가 일어난다.
// 하지만 데이터 관련 datakit 이 아직 만들어 지지 않았기 때문에 입력파라미터로 dag 수를 지정한다.
// 이부분에서 두가지를 생각할 수 있다. dag 하나를 받아들여서 늘리는 방향과 dag 는 하나이고 데이터만큼만 어떠한 방식으로 진행하는 것이다.
// 전자가 쉽게 생각할 수 있지만 메모리 낭비 가있다. 일단 전자로 개발한다. 후자는 아직 아이디어가 없다.
// TODO 데이터와 관련해서 추가 해서 수정해줘야 한다. 추후 안정화 되면 panic 은 error 로 교체한다.
func (pipe *Pipeline) NewDags(ds int, original *Dag) *Pipeline {
	/*n := 1
	pid := pipe.Id
	dags := len(pipe.Dags)

	if dags > 0 {
		n = dags + 1
	}
	sn := strconv.Itoa(n)*/
	//dag := NewDagWithPId(pid, sn)

	var dag *Dag
	if utils.IsEmptyString(pipe.Id) {
		panic("pipeline id is empty")
	}

	if ds < 1 {
		panic("input parameter is invalid")
	}

	for i := 1; i <= ds; i++ {
		dagId := fmt.Sprintf("%s-%d", pipe.Id, i)
		dag = NewDagWithPId(dagId, pipe.ContainerCmd)

		if dag == nil {
			panic("NewDagWithPId failed")
		}

		pipe.Dags = append(pipe.Dags, dag)
	}

	return pipe
}

func (pipe *Pipeline) SetContainerCmd(r Runnable) error {
	if r == nil {
		return fmt.Errorf("runnable is nil")
	}
	if pipe.ContainerCmd == nil {
		pipe.ContainerCmd = r
	}
	return nil
}
