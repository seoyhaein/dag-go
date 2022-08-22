package podbridge

import (
	"context"
	"errors"

	"github.com/containers/podman/v4/pkg/bindings/pods"
	"github.com/seoyhaein/utils"
)

type CreatePodResult struct {
	ErrorMessage error

	Name     string
	Hostname string
	ID       string

	success bool
}

func PodWithSpec(ctx context.Context, podConfig *PodConfig) *CreatePodResult {

	result := new(CreatePodResult)

	if podConfig.IsSetPodSpec() == utils.PFalse || podConfig.IsSetPodSpec() == nil {

		result.ErrorMessage = errors.New("PodSpec is not set")
		result.success = false
		return result
	}

	// 추가
	err := PodSpec.PodSpecGen.Validate()

	if err != nil {
		result.ErrorMessage = err
		result.success = false
		return result
	}

	podExists, err := pods.Exists(ctx, PodSpec.PodSpecGen.Name, &pods.ExistsOptions{})

	if err != nil {
		result.ErrorMessage = err
		result.success = false
		return result
	}

	// 기존에 pod 가 존재할 경우는 에러 리턴. 다른 시스템이 만들어 놓을 수 있기 때문에 타 pod 는 건드리지 않는다.
	if podExists {
		result.ErrorMessage = errors.New("the same Pod exists")
		result.success = false
		return result
	}

	podReport, err := pods.CreatePodFromSpec(ctx, PodSpec)

	if err != nil {
		result.ErrorMessage = err
		result.success = false
		return result
	}

	result.ErrorMessage = nil
	result.ID = podReport.Id
	result.Name = PodSpec.PodSpecGen.Name
	result.Hostname = PodSpec.PodSpecGen.Hostname
	result.success = true

	return result
}

func (podRes *CreatePodResult) GetPodId() string {
	return podRes.ID
}
