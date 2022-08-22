//This file now only builds on Linux.
//go:build linux
// +build linux

package podbridge

import (
	"context"
	"errors"
	"os"

	"github.com/containers/podman/v4/pkg/bindings"
	"github.com/seoyhaein/utils"
)

//TODO local 에 podman 설치가 되어 있는 경우만 구현했다. 추후 원격 연결도 확장해 나간다.

func NewConnection(ctx context.Context, ipcName string) (context.Context, error) {

	if utils.IsEmptyString(ipcName) {
		return nil, errors.New("ipcName cannot be an empty string")
	}
	cTx, err := bindings.NewConnection(ctx, ipcName)

	return cTx, err
}

func defaultLinuxSockDir() (socket string) {

	sockDir := os.Getenv("XDG_RUNTIME_DIR")
	if sockDir == "" {
		sockDir = "/var/run"
	}
	socket = "unix:" + sockDir + "/podman/podman.sock"

	return
}

func NewConnectionLinux(ctx context.Context) (context.Context, error) {

	socket := defaultLinuxSockDir()
	cTx, err := bindings.NewConnection(ctx, socket)

	return cTx, err
}
