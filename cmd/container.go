package cmd

import (
	"context"
	"fmt"

	dag "github.com/seoyhaein/dag-go"
	pdb "github.com/seoyhaein/podbridge"
)

func CreateCommand() *dag.Command {

	var cmd = &dag.Command{
		RunE: func() error {

			return nil
		},
	}

	return cmd
}

func InitCommand() (*context.Context, error) {
	cTx, err := pdb.NewConnectionLinux(context.Background())

	if err != nil {
		return nil, fmt.Errorf("check whether the podman-related dependencies and installation have been completed, " +
			"and the Run API service (Podman system service) has been started")
	}

	return cTx, nil
}

// 혹시 참고  할수 있을지 검토
// https://stackoverflow.com/questions/48263281/how-to-find-sshd-service-status-in-golang
