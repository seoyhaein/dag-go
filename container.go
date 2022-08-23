package dag_go

import (
	"context"
	"fmt"
	"time"

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

func (c *Container) RunE(imageName string) error {

	/*
			// TODO image 만들고 해야 함. 여기서 문제 발생할 듯 한데.. 흠..

			// basket


		// spec 만들기
		/*	conSpec := pbr.NewSpec()
			conSpec.SetImage("docker.io/library/test07")

			f := func(spec pbr.SpecGen) pbr.SpecGen {
				spec.Name = n.Id + "test"
				spec.Terminal = true
				return spec
			}
			conSpec.SetOther(f)
			// 해당 이미지에 해당 shell script 가 있다.
			conSpec.SetHealthChecker("CMD-SHELL /app/healthcheck/healthcheck.sh", "2s", 1, "30s", "1s")

			// container 만들기
			r := pbr.CreateContainer(c.context, conSpec)
			fmt.Println("container Id is :", r.ID)
			result := r.RunT(c.context, "1s")

			v := int(result)
			fmt.Println(v)*/
	fmt.Println("connect")
	time.Sleep(time.Second * 5)

	return fmt.Errorf("test error")
}

/*func CreateCommand() *dag.Command {

	var cmd = &dag.Command{
		RunE: func() error {

			return nil
		},
	}

	return cmd
}*/

// 혹시 참고  할수 있을지 검토
// https://stackoverflow.com/questions/48263281/how-to-find-sshd-service-status-in-golang
