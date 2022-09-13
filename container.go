package dag_go

import (
	"context"
	"fmt"
	"math/rand"

	pbr "github.com/seoyhaein/podbridge"
	"github.com/seoyhaein/utils"
)

// TODO buildah, types 는 imageV1 과 podbridge 에서 해결해줘야 함.
// 혹시 참고  할수 있을지 검토
// https://stackoverflow.com/questions/48263281/how-to-find-sshd-service-status-in-golang

type Container struct {
	Context   context.Context
	BaseImage string
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

/*
참고
	Created   ContainerStatus = iota //0
	Running                          // 1
	Exited                           // 2
	ExitedErr                        // 3
	Healthy                          // 4
	Unhealthy                        // 5
	Dead                             // 6
	Paused                           // 7
	UnKnown                          // 8
	None                             // 9
*/

// RunE 8 is 'None' -> check podbridge config.go
func (c *Container) RunE(a interface{}) (int, error) {

	n, ok := a.(*Node)
	if ok {
		r := createContainer(c.Context, n)
		// 정상적인 종료
		if r == 2 {
			return r, nil
		}
		if r == 3 {
			return r, fmt.Errorf("cannot execute container")
		}
		if r == 5 {
			return r, fmt.Errorf("unhealthy")
		}
		if r == 6 {
			return r, fmt.Errorf("container dead")
		}
		/*if r == 7 {
			return 7, fmt.Errorf("pause")
		}*/
		if r == 8 {
			return r, fmt.Errorf("node's ImageNme is empty")
		}
		if r == 9 {
			return r, fmt.Errorf("none")
		}
	}
	return 9, fmt.Errorf("none")
}

func (c *Container) RunET(a interface{}) (pbr.ContainerStatus, error) {

	n, ok := a.(*Node)
	if ok {
		r := createContainerT(c.Context, n)
		// 정상적인 종료
		if r == pbr.Exited {
			return r, nil
		}
		if r == pbr.ExitedErr {
			return r, fmt.Errorf("cannot execute container")
		}
		if r == pbr.Unhealthy {
			return r, fmt.Errorf("unhealthy")
		}
		if r == pbr.Dead {
			return r, fmt.Errorf("container dead")
		}
		/*if r == 7 {
			return 7, fmt.Errorf("pause")
		}*/
		if r == pbr.UnKnown {
			return r, fmt.Errorf("node's ImageNme is empty")
		}
		if r == pbr.UnKnown {
			return r, fmt.Errorf("none")
		}
	}
	return pbr.UnKnown, fmt.Errorf("none")
}

// CreateImage TODO healthCheckr 의 empty 검사만 하지만 실제로 healthchecker.sh 가 있는지 파악하는 구문 들어갈지 생각하자.
// 각 노드의 이미지 를만들어 줌.
func (c *Container) CreateImage(a interface{}, healthChecker string) error {
	n, ok := a.(*Node)
	if ok {
		if utils.IsEmptyString(healthChecker) {
			return fmt.Errorf("healthChecker is empty")
		}
		if utils.IsEmptyString(c.BaseImage) {
			c.BaseImage = pbr.CreateBaseImage(healthChecker)
		}
		randStr := randSeq(5)
		filename := fmt.Sprintf("%s%s", n.Id, randStr)

		nodeImage := pbr.CreateCustomImage(n.Id, c.BaseImage, filename, n.Commands)
		if nodeImage == nil {
			return fmt.Errorf("cannot create node image")
		}
		n.ImageName = *nodeImage
		return nil
	}
	return fmt.Errorf("cannot create node image")
}

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
	result := r.Run(ctx, "1s")

	v := int(result)
	return v
}

//createContainer 각 노드의 이미지를 가지고 container 를 만들어줌. TODO 수정한다.
func createContainerT(ctx context.Context, n *Node) pbr.ContainerStatus {
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
	result := r.Run(ctx, "1s")

	return result
}

// add by seoy
// https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go/22892986#22892986
var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
