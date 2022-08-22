package dag_go

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/containers/buildah"
	"github.com/containers/image/v5/types"
	pbr "github.com/seoyhaein/podbridge"
	v1 "github.com/seoyhaein/podbridge/imageV1"
	"github.com/seoyhaein/utils"
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

// "./healthcheck/executor.sh"
// "./healthcheck/healthcheck.sh"

// DefaultImage alpine 으로 만들어주고 명령어를 넣어줘서 image 를 만들어 주는 function.
// 1. bash, nano 와 업데이트를 해줌.
// 2. /app 폴더를 WorkDir 로 만들어줌.
// 3. /app/healthcheck 폴더를 만들어줌.
// 4. /app/healthcheck 여기에 executor.sh 를 집어 넣어줌.
func DefaultImage(exePath, healthCheckerPath, imageName, cmd string) *string {
	if utils.IsEmptyString(exePath) || utils.IsEmptyString(healthCheckerPath) || utils.IsEmptyString(imageName) {
		return nil
	}
	pbr.MustFirstCall()
	opt := v1.NewOption().Other().FromImage("alpine:latest")
	ctx, builder, err := v1.NewBuilder(context.Background(), opt)

	defer func() {
		builder.Shutdown()
		builder.Delete()
	}()

	if err != nil {
		Log.Printf("NewBuilder error")
		panic(err)
	}

	err = builder.Run("apk update")
	builder.Run("apk add --no-cache bash nano")
	if err != nil {
		Log.Printf("Run error")
		panic(err)
	}

	err = builder.WorkDir("/app")
	if err != nil {
		Log.Println("WorkDir")
		panic(err)
	}

	builder.Run("mkdir -p /app/healthcheck")
	if err != nil {
		Log.Println("Run error")
		panic(err)
	}
	// ADD/Copy 동일함.
	// executor.sh 추가 해줌.
	err = builder.Add(exePath, "/app/healthcheck")
	if err != nil {
		Log.Println("Add error")
		panic(err)
	}
	// add healthchecker.sh 추가해줌.
	err = builder.Add(healthCheckerPath, "/app/healthcheck")
	if err != nil {
		Log.Println("Add error")
		panic(err)
	}
	err = builder.Cmd("/app/healthcheck/executor.sh")
	if err != nil {
		Log.Println("cmd error")
		panic(err)
	}

	// TODO 이부분 향후 이 임포트 숨기는 방향으로 간다.
	sysCtx := &types.SystemContext{}
	image, err := builder.CommitImage(ctx, buildah.Dockerv2ImageManifest, sysCtx, imageName)

	if err != nil {
		Log.Println("CommitImage error")
		panic(err)
	}

	return image
}

//genExecutorSh 동일한 위치에 파일이 있으면 실패한다.
func genExecutorSh(path, fileName, cmd string) (*os.File, error) {
	if utils.IsEmptyString(path) || utils.IsEmptyString(fileName) {
		return nil, fmt.Errorf("path or file name is empty")
	}

	var (
		f   *os.File
		err error
	)

	defer func() {
		if err = f.Close(); err != nil {
			panic(err)
		}
	}()

	b, _, err := utils.FileExists(path)
	if err != nil {
		panic(err)
	}
	// 해당 위치에 파일이 없다면
	if b == false {
		f, err = os.Create(fileName)
		if err != nil {
			pbr.Log.Printf("cannot create file")
			panic(err)
		}

		sh := []byte(`#!/bin/bash
set -o pipefail -o errexit
echo "pid:"$$ | tee ./log
`)
		body := []byte(cmd)
		tail := []byte(`
echo "exit:"$? | tee ./log
`)
		sh = append(sh, body...)
		sh = append(sh, tail...)

		f.Write(sh)
		f.Sync()

		return f, nil
	}

	return nil, fmt.Errorf("cannot create file")
}

func createPodbridgeYaml() *os.File {
	var (
		f   *os.File
		err error
	)

	defer func() {
		if err = f.Close(); err != nil {
			panic(err)
		}
	}()

	f, err = os.Create("podbridge.yaml")
	if err != nil {
		return nil
	}
	return f
}

func (c *Container) RunE(d *Dag) error {

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
