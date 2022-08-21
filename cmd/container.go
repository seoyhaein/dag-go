package cmd

import (
	"context"
	"fmt"

	pbr "github.com/seoyhaein/podbridge"
)

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

func DefaultImage() {

}

func (c *Container) RunE() error {
	/*// TODO 바깥으로 빼자.
	ctx, err := pbr.NewConnectionLinux(context.Background())
	if err != nil {
		panic(err)
		//return 8, fmt.Errorf("check whether the podman-related dependencies and installation have been completed, " +
		//	"and the Run API service (Podman system service) has been started")
	}*/
	/*
		// TODO image 만들고 해야 함. 여기서 문제 발생할 듯 한데.. 흠..

		// basket
		pbr.MustFirstCall()
		opt := v1.NewOption().Other().FromImage("alpine:latest")
		ctx, builder, err := v1.NewBuilder(context.Background(), opt)

		defer func() {
			builder.Shutdown()
			builder.Delete()
		}()

		if err != nil {
			fmt.Println("NewBuilder")
			os.Exit(1)
		}
		err = builder.Run("apk update")
		builder.Run("apk add --no-cache bash nano")
		if err != nil {
			fmt.Println("Run1")
			os.Exit(1)
		}

		err = builder.WorkDir("/app")
		if err != nil {
			fmt.Println("WorkDir")
			os.Exit(1)
		}

		builder.Run("mkdir -p /app/healthcheck")
		if err != nil {
			fmt.Println("Run1")
			os.Exit(1)
		}
		// ADD/Copy 동일함.
		err = builder.Add("./healthcheck/executor.sh", "/app/healthcheck")
		if err != nil {
			fmt.Println("Add")
			os.Exit(1)
		}

		err = builder.Add("./healthcheck/healthcheck.sh", "/app/healthcheck")
		if err != nil {
			fmt.Println("Add")
			os.Exit(1)
		}
		err = builder.Cmd("/app/healthcheck/executor.sh")

		// TODO 이부분 향후 이 임포트 숨기는 방향으로 간다.
		sysCtx := &types.SystemContext{}
		image, err := builder.CommitImage(ctx, buildah.Dockerv2ImageManifest, sysCtx, "test07")

		if err != nil {
			fmt.Println("CommitImage")
			os.Exit(1)
		}

		fmt.Println(*image)*/

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
	return nil
}

/*func CreateCommand() *dag.Command {

	var cmd = &dag.Command{
		RunE: func() error {

			return nil
		},
	}

	return cmd
}*/

/*func InitCommand() (context.Context, error) {
	ctx, err := pbr.NewConnectionLinux(context.Background())

	if err != nil {
		return nil, fmt.Errorf("check whether the podman-related dependencies and installation have been completed, " +
			"and the Run API service (Podman system service) has been started")
	}

	return ctx, nil
}*/

// 혹시 참고  할수 있을지 검토
// https://stackoverflow.com/questions/48263281/how-to-find-sshd-service-status-in-golang
