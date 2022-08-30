package podbridge

import (
	"context"
	"fmt"
	"os"

	"github.com/containers/buildah"
	"github.com/containers/image/v5/types"
	v1 "github.com/seoyhaein/podbridge/imageV1"
	"github.com/seoyhaein/utils"
)

// CreateCustomImage alpine 으로 만들어주고 명령어를 넣어줘서 image 를 만들어 주는 function.
// 테스트는 별도의 프로젝트 열어서 해야함.
// 1. bash, nano 와 업데이트를 해줌.
// 2. /app 폴더를 WorkDir 로 만들어줌.
// 3. /app/healthcheck 폴더를 만들어줌.
// 4. /app/healthcheck 여기에 executor.sh 를 집어 넣어줌.
// TODO 데이터 넣는 것도 구현되어야 함. BaseImage 를 만들자.
func CreateCustomImage(imageName, baseImage, cmd string) *string {
	if utils.IsEmptyString(imageName) {
		return nil
	}
	var executorPath *string
	// 이미지 만들고 생성한 file 삭제
	// TODO MustFristCall 중복 호출에 대한 부분 check.
	MustFirstCall()

	if utils.IsEmptyString(baseImage) {
		panic("baseImage is nil")
	}

	opt := v1.NewOption().Other().FromImage(baseImage)
	ctx, builder, err := v1.NewBuilder(context.Background(), opt)

	defer func() {
		builder.Shutdown()
		builder.Delete()
	}()

	if err != nil {
		Log.Printf("NewBuilder error")
		panic(err)
	}

	err = builder.WorkDir("/app")
	if err != nil {
		Log.Println("WorkDir")
		panic(err)
	}

	// ADD/Copy 동일함.
	// executor.sh 추가 해줌.
	// TODO 여기서 고정으로 filename 고정시켜줘서 문제가 발생함.
	// 컨테이너 안에서 바꿔주자.
	_, executorPath, _ = genExecutorSh(".", "executor.sh", cmd)
	defer func(path string) {
		err := os.Remove(path)
		if err != nil {
			panic(err)
		}
	}(*executorPath)

	err = builder.Add(*executorPath, "/app/healthcheck")
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

func CreateCustomImageT(imageName, baseImage, filename, cmd string) *string {
	if utils.IsEmptyString(imageName) || utils.IsEmptyString(filename) {
		return nil
	}
	var executorPath *string
	// 이미지 만들고 생성한 file 삭제
	// TODO MustFristCall 중복 호출에 대한 부분 check.
	MustFirstCall()

	if utils.IsEmptyString(baseImage) {
		panic("baseImage is nil")
	}

	opt := v1.NewOption().Other().FromImage(baseImage)
	ctx, builder, err := v1.NewBuilder(context.Background(), opt)

	defer func() {
		builder.Shutdown()
		builder.Delete()
	}()

	if err != nil {
		Log.Printf("NewBuilder error")
		panic(err)
	}

	err = builder.WorkDir("/app")
	if err != nil {
		Log.Println("WorkDir")
		panic(err)
	}

	// ADD/Copy 동일함.
	// executor.sh 추가 해줌.
	// TODO 여기서 고정으로 filename 고정시켜줘서 문제가 발생함.
	// 컨테이너 안에서 바꿔주자.
	_, executorPath, _ = genExecutorSh(".", filename, cmd)
	defer func(path string) {
		err := os.Remove(path)
		if err != nil {
			panic(err)
		}
	}(*executorPath)

	err = builder.Add(*executorPath, "/app/healthcheck")
	if err != nil {
		Log.Println("Add error")
		panic(err)
	}

	subCmd := fmt.Sprintf("mv /app/healthcheck/%s executor.sh", filename)
	err = builder.Run(subCmd)
	if err != nil {
		Log.Println("Run error")
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

// genExecutorSh 동일한 위치에 파일이 있으면 실패한다.
// TODO performance test 하자.
// 기존에 파일이 있으면 에러난다. 이문제는 goroutine 에서도 문제될듯하다. 그래서 계속 실패한듯.
// 파일이름을 unique 하게 바꿔줘야 겠다.
func genExecutorSh(path, fileName, cmd string) (*os.File, *string, error) {
	if utils.IsEmptyString(path) || utils.IsEmptyString(fileName) {
		return nil, nil, fmt.Errorf("path or file name is empty")
	}
	var (
		f   *os.File
		err error
	)
	defer func() {
		if f == nil {
			return
		}
		if err = f.Close(); err != nil {
			panic(err)
		}
	}()
	executorPath := fmt.Sprintf("%s/%s", path, fileName)
	b, _, err := utils.FileExists(executorPath)

	if err != nil {
		panic(err)
	}
	// 해당 위치에 파일이 없다면
	if b == false {
		f, err = os.Create(fileName)
		if err != nil {
			Log.Printf("cannot create file")
			panic(err)
		}
		// 파일 권한 다줌.
		err = os.Chmod(fileName, 0777)
		if err != nil {
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

		return f, &executorPath, nil
	}
	return nil, nil, fmt.Errorf("cannot create file")
}

// CreateBaseImage healthcheck 를 넣는다.
func CreateBaseImage(healthCheckerPath string) string {

	MustFirstCall()
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

	// 파일권한 다줌.
	err = os.Chmod(healthCheckerPath, 0777)
	if err != nil {
		panic(err)
	}

	// add healthcheck.sh 추가해줌.
	err = builder.Add(healthCheckerPath, "/app/healthcheck")
	if err != nil {
		Log.Println("Add error")
		panic(err)
	}

	// TODO 이부분 향후 이 임포트 숨기는 방향으로 간다.
	sysCtx := &types.SystemContext{}
	image, err := builder.CommitImage(ctx, buildah.Dockerv2ImageManifest, sysCtx, "custombaseimage")

	if err != nil {
		Log.Println("CommitImage error")
		panic(err)
	}

	return *image
}

// https://stackoverflow.com/questions/60128401/how-to-check-if-a-file-is-executable-in-go
// https://golangbyexample.com/change-file-permissions-golang/
