package localmachine

import (
	"fmt"
	"os/exec"
)

// container status
// https://blog.naver.com/alice_k106/221310477844
// https://docs.podman.io/en/latest/markdown/podman-container-exists.1.html

// https://stackoverflow.com/questions/48263281/how-to-find-sshd-service-status-in-golang

// 아래와 같은 방식으로 제작한다. 다만 리턴에 대한 문제는 생가해봐야 한다.

// 중요!! 지속적으로 container 의 status 를 확인해줘야 함으로, goroutine, 루프 구문이 들어가고 context 가 들어가고, channel 이 들어가야 할듯 하다.
func ContainerExists(id string) error {
	cmd := exec.Command("podman", "container", "exists", id)

	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Errorf("Cannot find process")

	}
	fmt.Printf("Status is: %s", string(out))
	fmt.Println("Starting Role")

	return nil
}
