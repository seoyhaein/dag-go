### example

```
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/containers/buildah"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage/pkg/unshare"
	pbr "github.com/seoyhaein/podbridge"
	v1 "github.com/seoyhaein/podbridge/imageV1"
)

func main() {
	if buildah.InitReexec() {
		return
	}
	unshare.MaybeReexecUsingUserNamespace(false)

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

	/*	err = builder.User("root")
		if err != nil {
			fmt.Println("User")
			os.Exit(1)
		}*/

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

	/*err = builder.Run("./file.sh")
	if err != nil {
		fmt.Println("Run2")
		os.Exit(1)
	}*/

	/*err = builder.Run("yarn install --production")
	if err != nil {
		fmt.Println("Run2")
		os.Exit(1)
	}*/

	err = builder.Cmd("/app/healthcheck/executor.sh")

	/*err = builder.Expose("3000")
	if err != nil {
		fmt.Println("Expose")
		os.Exit(1)
	}*/

	sysCtx := &types.SystemContext{}
	image, err := builder.CommitImage(ctx, buildah.Dockerv2ImageManifest, sysCtx, "test07")

	if err != nil {
		fmt.Println("CommitImage")
		os.Exit(1)
	}

	fmt.Println(*image)

}

```

### TODO
- types.SystemContext 관련해서 소스 분석해서 자세한 내용 파악.