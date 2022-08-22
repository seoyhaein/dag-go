package podbridge

import (
	"fmt"
	"runtime"

	"github.com/containers/podman/v4/pkg/util"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// TODO 잠시 미루고, containerfile, 생성 및 command, heartbeat?? 관련 완성한 다음에 진행하자.
// 테스트 할 방법이 예매함.
// 현재 os 의 resource  를 가지고 와서 이것과 비교해서 제한을 둬야 한다.
// mesos resource 참고

// https://access.redhat.com/documentation/ko-kr/red_hat_enterprise_linux/6/html/resource_management_guide/sec-cpu

// cpu share 의 개념
// https://kimmj.github.io/kubernetes/kubernetes-cpu-request-limit/

// http://jake.dothome.co.kr/control-groups/
// https://m.blog.naver.com/complusblog/220994619068

// TODO 참고해서 작성해야 함.
// https://pkg.go.dev/github.com/shirou/gopsutil@v3.21.11+incompatible/docker

// 잠깐 조사를 거친 후 드는 생각은 mesos 의 리소스 설정의 경우는 cpu 는 코어 만 적용이되는 듯하다. 이 부분은 실제로 테스트 하면서 많이 조사를 해야 하는 부분이다.
// nomad 참고하자. 이녀석은 specs-go 를 참고하지 않고 자체적으로 struct 를 구현했다. => 표준과 벗어난 거 아닌가?? 흠.

// nomad driver_test.go 내용 참고하자.
/*
var (
	basicResources = &drivers.Resources{
		NomadResources: &structs.AllocatedTaskResources{
			Memory: structs.AllocatedMemoryResources{
				MemoryMB: 256,
			},
			Cpu: structs.AllocatedCpuResources{
				CpuShares: 250,
			},
		},
		LinuxResources: &drivers.LinuxResources{
			CPUShares:        512,
			MemoryLimitBytes: 256 * 1024 * 1024,
		},
	}
)
*/

// 위의 내용을 살펴보면, mesos 에서는 resource 관련해서는 container 와 non-container 의 리소스 설정이 다르다.
// https://mesos.apache.org/documentation/attributes-resources/ -> non-container
// https://mesos.apache.org/documentation/latest/nested-container-and-task-group/ -> container

// TODO error prone!
// return specs.LinuxResource 로 리턴해야 함.

// 참고
// convert
// https://www.gbmb.org/gigabytes

func LimitResources(cores float64, mems float64) (*specs.LinuxCPU, *specs.LinuxResources) {

	if cores <= 0 {
		return nil, nil
	}

	LinuxCpus := new(specs.LinuxCPU)

	period, quota := util.CoresToPeriodAndQuota(cores)
	LinuxCpus.Period = &period
	LinuxCpus.Quota = &quota
	LinuxCpus.Cpus = fmt.Sprintf("%f", cores)
	LinuxCpus.Mems = fmt.Sprintf("%f", mems)

	return LinuxCpus, nil
}

/*func LimitResourceV2() *specs.LinuxResources {

}*/

// 특정 코어에 제한을 하는 방법과, 전체 cpu 와 mem 을 제한하는 방법이 있다. 일단 두가지다 제공해주는 방법을 사용하되, 혼용해서 쓰는 것은 제한한다.
// 1 GiB = 1024 MiB = 1024 * 1024 KiB = 1024 * 1024 * 1024 B

func getCores(isLogical bool) (uint8, error) {
	cores, err := cpu.Counts(isLogical)

	if err != nil {
		return 0, err
	}

	cpus := runtime.NumCPU()

	fmt.Printf("cpus:%d, cores:%d \n", cpus, cores)

	return uint8(cores), nil
}

func getTotalUsedMem() (uint64, uint64, error) {
	v, err := mem.VirtualMemory()

	if err != nil {
		return 0, 0, err
	}
	return v.Total, v.Used, nil

}

// 테스트 해야 함.
// 0 번째 코어에 512kb 사용하는 방식임.
// 고생좀 할 것 같다.

func SetCpuSet() *specs.LinuxResources {

	cpuset := new(specs.LinuxResources)

	cpuset.CPU.Cpus = "0"
	cpuset.CPU.Mems = "512*1024*1024"

	return cpuset
}

// 특정 컨테이너가 특정 코어를 사용하는지 파악해야 한다.
