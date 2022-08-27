package podbridge

import (
	"fmt"
	"time"

	"github.com/containers/podman/v4/pkg/domain/entities"
	"github.com/containers/podman/v4/pkg/specgen"
	"github.com/seoyhaein/utils"
	"github.com/sirupsen/logrus"
)

var Log = logrus.New()

// pod, spec 삭제할 예정임.
// TODO 코드 정리가 필요하다.
type (
	ContainerConfig struct {
		SetSpec                 *bool
		AutoCreateContainerName *bool
	}

	PodConfig struct {
		SetPodSpec                   *bool
		AutoCreatePodNameAndHostName *bool
	}
)

func (conf *ContainerConfig) TrueSetSpec() *bool {

	conf.SetSpec = utils.PTrue
	return conf.SetSpec
}

func (conf *ContainerConfig) FalseSetSpec() {
	conf.SetSpec = utils.PFalse
	conf.AutoCreateContainerName = utils.PFalse
}

func (conf *ContainerConfig) IsSetSpec() *bool {

	return conf.SetSpec
}

func (conf *ContainerConfig) IsAutoCreateContainerName() *bool {

	return conf.AutoCreateContainerName
}

// 이름을 자동 설정하고 이 메서드를 호출한다.
// 에러 조심하자. nil 의 의미.
// TODO 사용하는데 불편함이 있다. 추후 수정.

func (conf *ContainerConfig) TrueAutoCreateContainerName(spec *specgen.SpecGenerator) *bool {

	// string 이 empty 이면, 즉 세팅이 안되어 있으면
	if utils.IsEmptyString(spec.Name) {
		conf.createSpecContainerName()
		conf.AutoCreateContainerName = utils.PTrue
		return conf.AutoCreateContainerName
	} else { // 만약 Spec.Name 이 세팅되어 있으면 nil 반환.
		if conf.AutoCreateContainerName == utils.PTrue {
			conf.AutoCreateContainerName = utils.PFalse
		}

		return nil
	}
}

func (conf *ContainerConfig) FalseAutoCreateContainerName() {
	conf.AutoCreateContainerName = utils.PFalse
}

// TODO apis.go 로 이동 및 옵션을 만들어서 이름을 자동으로 만들어 줄지 설정할 수 있도록 한다.
// 일단 최초 컨테이너가 생성된 시점의 시간을 기록한다.
// 추가적으로 기록될 필요가 있는 정보가 있으면 추가한다.
// TODO 메서드로 처리하는게 맞는지 생각하기.

func (conf *ContainerConfig) createSpecContainerName() {
	Spec.Name = time.Now().Format("20220702-15h04m05s")
}

// pod

func (podConf *PodConfig) TrueSetPodSpec() *bool {

	podConf.SetPodSpec = utils.PTrue
	return podConf.SetPodSpec
}

func (podConf *PodConfig) FalseSetPodSpec() {
	podConf.SetPodSpec = utils.PFalse
	podConf.AutoCreatePodNameAndHostName = utils.PFalse
}

func (podConf *PodConfig) IsSetPodSpec() *bool {

	return podConf.SetPodSpec
}

func (podConf *PodConfig) IsAutoCreatePodNameAndHost() *bool {

	return podConf.AutoCreatePodNameAndHostName
}

func (podConf *PodConfig) TrueAutoCreatePodNameAndHost(podspec *entities.PodSpec) *bool {

	// string 이 empty 이면, 즉 세팅이 안되어 있으면
	if utils.IsEmptyString(podspec.PodSpecGen.Name) || utils.IsEmptyString(podspec.PodSpecGen.Hostname) {
		podConf.createSpecPodNameAndHost()
		podConf.AutoCreatePodNameAndHostName = utils.PTrue
		return podConf.AutoCreatePodNameAndHostName
	} else { // 만약 Spec.Name 이 세팅되어 있으면 nil 반환.
		if podConf.AutoCreatePodNameAndHostName == utils.PTrue {
			podConf.AutoCreatePodNameAndHostName = utils.PFalse
		}

		return nil
	}
}

func (podConf *PodConfig) FalseAutoCreatePodNameAndHost() {
	podConf.AutoCreatePodNameAndHostName = utils.PFalse
}

// TODO 메서드로 처리하는게 맞는지 생각하기.
func (podConf *PodConfig) createSpecPodNameAndHost() {
	// TODO 날짜 안나오는 에러 수정
	PodSpec.PodSpecGen.Name = fmt.Sprintf("pod-%s", time.Now().Format("20220702-15h04m05s"))
	PodSpec.PodSpecGen.Hostname = "IchthysGenomics"
}

type ContainerStatus int

const (
	Created   ContainerStatus = iota //0
	Running                          // 1
	Exited                           // 2
	ExitedErr                        // 3
	Healthy                          // 4
	Unhealthy                        // 5
	Dead                             // 6
	Paused                           // 7
	UnKnown                          // 8
	None                             // 9
)

// channel buffer size
const Max int = 100
