package podbridge

import (
	"context"

	"github.com/containers/podman/v4/pkg/bindings/volumes"
	"github.com/containers/podman/v4/pkg/domain/entities"
	"github.com/containers/podman/v4/pkg/specgen"
	"github.com/seoyhaein/utils"
)

type VolumeConfig struct {
	entities.VolumeCreateOptions
	Dest string
}

// TODO 테스트
// 조심해서 살펴보자.
// 사용되는 기술은 아니지만 참고해보자.
// https://dev.to/chattes/s3-as-docker-volumes-3bkd

// https://blog.meain.io/2020/mounting-s3-bucket-kube/

// 하나라도 에러나면 에러 리턴 하고 끝남.
// volume 을 만들어 주고, NamedVolume 으로 다시 만들어 준다.

func CreateNamedVolume(ctx context.Context, conf ...*VolumeConfig) ([]*specgen.NamedVolume, error) {
	var results []*specgen.NamedVolume
	for _, v := range conf {
		volumeConfigResponse, err := volumes.Create(ctx, v.VolumeCreateOptions, &volumes.CreateOptions{})

		if err == nil {
			return nil, err
		}

		namedVol := new(specgen.NamedVolume)

		namedVol.Name = volumeConfigResponse.Name
		namedVol.Dest = v.Dest
		// TODO option setting
		results = append(results, namedVol)
	}
	return results, nil
}

// TODO volume 이 있으면 skip 하는 루틴 만들어야 함.

func (vo *VolumeConfig) genVolumeCreateOptions(name string, driver string, dest string, a ...any) *VolumeConfig {

	if utils.IsEmptyString(name) || utils.IsEmptyString(driver) || utils.IsEmptyString(dest) {
		return nil
	}

	vo.Name = name
	vo.Driver = driver
	vo.Dest = dest

	// TODO option setting
	return vo

}
