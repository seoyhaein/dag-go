package dag_go

import (
	"context"
	"fmt"

	pbr "github.com/seoyhaein/podbridge"
	"github.com/seoyhaein/utils"
)

// TODO buildah, types 는 imageV1 과 podbridge 에서 해결해줘야 함.
// 혹시 참고  할수 있을지 검토
// https://stackoverflow.com/questions/48263281/how-to-find-sshd-service-status-in-golang

type Container struct {
	Context context.Context
}

func Connect() *Container {
	ctx, err := pbr.NewConnectionLinux(context.Background())
	if err != nil {
		panic(err)
	}

	return &Container{
		Context: ctx,
	}
}

// RunE 8 is 'None' -> check podbridge config.go
func (c *Container) RunE(a interface{}) (int, error) {

	n, ok := a.(*Node)
	if ok {
		r := createContainer(c.Context, n)
		if r == 8 {
			return 8, fmt.Errorf("node's ImageNme is empty")
		}
		return r, nil
	}
	return 8, fmt.Errorf("RunE failed")
}

// CreateImage TODO healthCheckr 의 empty 검사만 하지만 실제로 healthchecker.sh 가 있는지 파악하는 구문 들어갈지 생각하자.
// 각 노드의 이미지 를만들어 줌.
func (c *Container) CreateImage(a interface{}, healthChecker string) error {
	n, ok := a.(*Node)
	if ok {
		if utils.IsEmptyString(healthChecker) {
			return fmt.Errorf("healthChecker is empty")
		}
		base := pbr.CreateBaseImage(healthChecker)
		nodeImage := pbr.CreateCustomImageB(n.Id, base, `echo "hello world"`)
		if nodeImage == nil {
			fmt.Errorf("cannot create node image")
		}
		n.ImageName = *nodeImage
	}
	return nil
}

// 각 노드의 이미지를 가지고 container 를 만들어줌.
func createContainer(ctx context.Context, n *Node) int {
	// spec 만들기
	conSpec := pbr.NewSpec()
	if utils.IsEmptyString(n.ImageName) {
		return 8
	}
	conSpec.SetImage(n.ImageName)

	f := func(spec pbr.SpecGen) pbr.SpecGen {
		spec.Name = n.Id + "test"
		spec.Terminal = true
		return spec
	}
	conSpec.SetOther(f)
	// 해당 이미지에 해당 shell script 가 있다.
	conSpec.SetHealthChecker("CMD-SHELL /app/healthcheck/healthcheck.sh", "2s", 1, "30s", "1s")

	// container 만들기
	r := pbr.CreateContainer(ctx, conSpec)
	fmt.Println("container Id is :", r.ID)
	result := r.RunT(ctx, "1s")

	v := int(result)
	return v
}
