package dag_go

import (
	"context"
	"fmt"
	pbr "github.com/seoyhaein/podbridge"
)

// TODO buildah, types 는 imageV1 과 podbridge 에서 해결해줘야 함.

type Container struct {
	context context.Context
}

func Connect() *Container {
	ctx, err := pbr.NewConnectionLinux(context.Background())
	if err != nil {
		panic(err)
	}

	return &Container{
		context: ctx,
	}
}

// RunE 8 is 'None' -> check podbridge config.go
func (c *Container) RunE(a interface{}) (int, error) {

	n, ok := a.(*Node)
	if ok {
		r := createContainer(c.context, n)
		return r, nil
	}
	return 8, fmt.Errorf("RunE failed")
}

func createContainer(ctx context.Context, n *Node) int {
	// spec 만들기
	conSpec := pbr.NewSpec()
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

// 혹시 참고  할수 있을지 검토
// https://stackoverflow.com/questions/48263281/how-to-find-sshd-service-status-in-golang
