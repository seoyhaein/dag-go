package podbridge

import (
	"context"
	"fmt"
	"time"

	"github.com/containers/podman/v4/pkg/bindings/containers"
	"github.com/containers/podman/v4/pkg/bindings/images"
	"github.com/containers/podman/v4/pkg/specgen"
	"github.com/seoyhaein/utils"
)

type (
	SpecGen *specgen.SpecGenerator

	CreateContainerResult struct {
		Name     string
		ID       string
		Warnings []string
		Status   ContainerStatus
		ch       chan ContainerStatus
	}

	ContainerSpec struct {
		Spec *specgen.SpecGenerator
	}
)

// NewSpec
func NewSpec() *ContainerSpec {
	return &ContainerSpec{
		Spec: new(specgen.SpecGenerator),
	}
}

// SetImage
func (c *ContainerSpec) SetImage(imgName string) *ContainerSpec {
	if utils.IsEmptyString(imgName) {
		return nil
	}
	spec := specgen.NewSpecGenerator(imgName, false)
	c.Spec = spec
	return c
}

// SetOther
func (c *ContainerSpec) SetOther(f func(spec SpecGen) SpecGen) *ContainerSpec {
	c.Spec = f(c.Spec)
	return c
}

// SetHealthChecker
func (c *ContainerSpec) SetHealthChecker(inCmd, interval string, retries uint, timeout, startPeriod string) *ContainerSpec {
	// cf. SetHealthChecker("CMD-SHELL /app/healthcheck.sh", "2s", 3, "30s", "1s")
	healthConfig, err := SetHealthChecker(inCmd, interval, retries, timeout, startPeriod)
	if err != nil {
		panic(err)
	}
	c.Spec.HealthConfig = healthConfig
	return c
}

// CreateContainer
// TODO 수정해줘야 함. name, image 확인해야 할듯, 일단 체크 해보자.
func CreateContainer(ctx context.Context, conSpec *ContainerSpec) *CreateContainerResult {
	var (
		result                 *CreateContainerResult
		containerExistsOptions containers.ExistsOptions
	)
	result = new(CreateContainerResult)
	err := conSpec.Spec.Validate()
	if err != nil {
		panic(err)
	}
	containerExistsOptions.External = utils.PFalse
	containerExists, err := containers.Exists(ctx, conSpec.Spec.Name, &containerExistsOptions)
	if err != nil {
		panic(err)
	}
	// 컨테이너가 local storage 에 존재하고 있다면
	if containerExists {
		var containerInspectOptions containers.InspectOptions
		containerInspectOptions.Size = utils.PFalse
		containerData, err := containers.Inspect(ctx, conSpec.Spec.Name, &containerInspectOptions)
		if err != nil {
			panic(err)
		}
		if containerData.State.Running {
			Log.Infof("%s container already running", conSpec.Spec.Name)
			result.ID = containerData.ID
			result.Name = conSpec.Spec.Name
			result.Status = Running
			return result
		} else {
			Log.Infof("%s container already exists", conSpec.Spec.Name)
			result.ID = containerData.ID
			result.Name = conSpec.Spec.Name
			result.Status = Created
			return result
		}
	} else {
		imageExists, err := images.Exists(ctx, conSpec.Spec.Image, nil)
		if err != nil {
			panic(err)
		}
		// TODO basket 에 넣을지 고민하자.
		if imageExists == false {
			_, err := images.Pull(ctx, conSpec.Spec.Image, &images.PullOptions{})
			if err != nil {
				panic(err)
			}
		}
		Log.Infof("Pulling %s image...\n", conSpec.Spec.Image)
		createResponse, err := containers.CreateWithSpec(ctx, conSpec.Spec, &containers.CreateOptions{})
		if err != nil {
			panic(err)
		}
		Log.Infof("Creating %s container using %s image...\n", conSpec.Spec.Name, conSpec.Spec.Image)
		result.Name = conSpec.Spec.Name
		result.ID = createResponse.ID
		result.Warnings = createResponse.Warnings
		result.Status = Created
	}
	if Basket != nil {
		Basket.AddContainerId(result.ID)
	}
	return result
}

// Start
// startOptions 는 default 값을 사용한다.
// https://docs.podman.io/en/latest/_static/api.html?version=v4.1#operation/ContainerStartLibpod
func (Res *CreateContainerResult) Start(ctx context.Context) error {
	if utils.IsEmptyString(Res.ID) == false && Res.Status == Created {
		err := containers.Start(ctx, Res.ID, &containers.StartOptions{})
		return err
	} else {
		return fmt.Errorf("cannot start container")
	}
}

// ReStart 중복되는 것 같긴하다. 수정해줘야 한다. ReStart
func (Res *CreateContainerResult) ReStart(ctx context.Context) error {
	if utils.IsEmptyString(Res.ID) == false && Res.Status != Running {
		err := containers.Start(ctx, Res.ID, &containers.StartOptions{})
		return err
	} else {
		return fmt.Errorf("cannot re-start container")
	}
}

// Stop
// TODO 추후 수정하자.
// https://docs.podman.io/en/latest/_static/api.html?version=v4.1#operation/ContainerStopLibpod
// default 값은 timeout 은  10 으로 세팅되어 있고, ignore 는 false 이다.
// ignore 는 만약 stop 된 컨테이너를 stop 되어 있을 때 stop 하는 경우 true 하면 에러 무시, false 로 하면 에러 리턴
// timeout 은 몇 후에 컨테어너를 kill 할지 정한다.
func (Res *CreateContainerResult) Stop(ctx context.Context, options ...any) error {
	stopOption := new(containers.StopOptions)
	for _, op := range options {
		v, b := op.(*bool)
		if b {
			stopOption.Ignore = v
		} else {
			v1, b1 := op.(uint)
			if b1 {
				stopOption.Timeout = &v1
			}
		}
	}

	err := containers.Stop(ctx, Res.ID, stopOption)
	return err
}

// Kill
func (Res *CreateContainerResult) Kill(ctx context.Context, options ...any) error {

	return nil
}

// HealthCheck 테스트 필요
func (Res *CreateContainerResult) HealthCheck(ctx context.Context, interval string) {
	if Res.ch == nil {
		Res.ch = make(chan ContainerStatus, Max)
	}

	go func(ctx context.Context, res *CreateContainerResult) {
		var containerInspectOptions containers.InspectOptions
		containerInspectOptions.Size = utils.PFalse

		containerData, err := containers.Inspect(ctx, res.ID, &containerInspectOptions)
		if err != nil {
			close(res.ch)
			return
		}
		if containerData.State.Dead {
			res.ch <- Dead
			close(res.ch)
			return
		}
		if containerData.State.Paused {
			res.ch <- Paused
			close(res.ch)
			return
		}

		intervalDuration, err := time.ParseDuration(interval)
		if err != nil {
			intervalDuration = time.Second
		}
		ticker := time.Tick(intervalDuration)
		for {
			select {
			case <-ticker:
				healthCheck, err := containers.RunHealthCheck(ctx, res.ID, &containers.HealthCheckOptions{})
				if err != nil {
					containerData, err = containers.Inspect(ctx, res.ID, &containerInspectOptions)
					if err != nil {
						fmt.Println(err.Error())
						res.ch <- UnKnown
						close(res.ch)
						return
					}
					if containerData.State.Dead {
						res.ch <- Dead
						close(res.ch)
						return
					}
					if containerData.State.Paused {
						res.ch <- Paused
						close(res.ch)
						return
					}
					res.ch <- Exited
					close(res.ch)
					return
				} else { // running 상태
					if healthCheck.Status == "healthy" {
						res.ch <- Healthy
					}
					if healthCheck.Status == "unhealthy" {
						res.ch <- Unhealthy
					}
				}
			case <-ctx.Done():
				close(res.ch)
				Log.Printf("cancel:sender")
				return
			}
		}
	}(ctx, Res)
}

// Run 테스트 필요
func (Res *CreateContainerResult) Run(ctx context.Context, interval string) <-chan ContainerStatus {
	out := make(chan ContainerStatus, Max)
	err := Res.Start(ctx)
	if err != nil {
		panic(err)
	}
	Res.HealthCheck(ctx, interval)

	go func(in chan ContainerStatus, out chan ContainerStatus) {
		defer close(out)
		for c := range in {
			if c == Unhealthy {
				out <- Unhealthy
			}
			if c == Healthy {
				out <- Healthy
			}
			if c == UnKnown {
				out <- UnKnown
			}
			if c == Exited {
				out <- Exited
			}
			if c == Dead {
				out <- Dead
			}
			if c == Paused {
				out <- Paused
			}
			out <- None
		}
	}(Res.ch, out)

	return out
}

func (Res *CreateContainerResult) RunT(ctx context.Context, interval string) ContainerStatus {
	err := Res.Start(ctx)
	if err != nil {
		panic(err)
	}
	Res.HealthCheck(ctx, interval)
	// 종료 상황일때만 리턴을 해줌.
	for c := range Res.ch {
		if c == Unhealthy {
			return Unhealthy
		}
		/*if c == Healthy {
			return Healthy
		}*/
		if c == UnKnown {
			return UnKnown
		}
		if c == Exited {
			return Exited
		}
		if c == Dead {
			return Dead
		}
		/*if c == Paused {
			return Paused
		}*/
	}
	return None
}

// 이미지 가존재하는지 확인하는 메서드 빼놓자.
// TODO wait 함수 구체적으로 살펴보기기
// 나머지들은 조금씩 구현해 나간다.
// containers.go
// TODO 중요 resource 관련
// https://github.com/containers/podman/issues/13145
// 명령어에 대한 heartbeat 관련 해서 처리 해야함.
// TODO 컨테이너의 상태를 확인하는 방법은 두가지 접근 방법이 있는데, local에 podman 이 설치 되어 있는 경우와, 원격(접속하는 머신에는 podman  이없음)에서 연결되는 경우
// 일단 먼저, local 에서 연결 하는 걸 적용한다. 구현하는 건 비교적 간단할 듯하다.
