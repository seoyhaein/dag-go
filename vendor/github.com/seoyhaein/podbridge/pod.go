package podbridge

import (
	"context"
	"errors"

	"github.com/containers/podman/v4/pkg/bindings/pods"
	"github.com/containers/podman/v4/pkg/domain/entities"
	"github.com/containers/podman/v4/pkg/specgen"
	"github.com/seoyhaein/utils"
)

type (
	PodSpecGen *specgen.PodSpecGenerator

	CreatePodResult struct {
		ErrorMessage error

		Name     string
		Hostname string
		ID       string

		Success bool
	}

	PodSpec struct {
		Spec *specgen.PodSpecGenerator
	}
)

// NewPod PodSpec 을 생성한다.
func NewPod() *PodSpec {
	return &PodSpec{
		Spec: new(specgen.PodSpecGenerator),
	}
}

// SetName Pod 의 이름을 등록한다.
func (c *PodSpec) SetName(name string) *PodSpec {
	if utils.IsEmptyString(name) {
		return nil
	}

	c.Spec.Name = name
	return c
}

// SetOther Spec 옵션을 적용하도록 돕는다.
func (c *PodSpec) SetOther(f func(spec PodSpecGen) PodSpecGen) *PodSpec {
	c.Spec = f(c.Spec)
	return c
}

// CreatePod Pod 를 생성한다. 기존 Pod 가 존재하고 있으면 실패를 리턴한다.
func CreatePod(ctx context.Context, spec *entities.PodSpec) *CreatePodResult {
	result := new(CreatePodResult)
	err := spec.PodSpecGen.Validate()

	if err != nil {
		result.ErrorMessage = err
		result.Success = false
		return result
	}

	podExists, err := pods.Exists(ctx, spec.PodSpecGen.Name, &pods.ExistsOptions{})

	if err != nil {
		result.ErrorMessage = err
		result.Success = false
		return result
	}

	// 기존에 pod 가 존재할 경우는 에러 리턴. 다른 시스템이 만들어 놓을 수 있기 때문에 타 pod 는 건드리지 않는다.
	if podExists {
		result.ErrorMessage = errors.New("the same Pod exists")
		result.Success = false
		return result
	}

	podReport, err := pods.CreatePodFromSpec(ctx, spec)

	if err != nil {
		result.ErrorMessage = err
		result.Success = false
		return result
	}

	result.ErrorMessage = nil
	result.ID = podReport.Id
	result.Name = spec.PodSpecGen.Name
	result.Hostname = spec.PodSpecGen.Hostname
	result.Success = true

	return result
}

func (podRes *CreatePodResult) GetPodId() string {
	return podRes.ID
}
