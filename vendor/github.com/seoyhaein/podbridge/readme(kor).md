# podbridge 한글 메뉴얼.
- 설치 관련해서는 podman_install.sh 을 참고 

## TODO
0. containerfile 만들어주는 부분 (이건 일단 후순위로 미룬다.) -> 필요 없을 듯 하다.
1. containerfile 에서 이미지 만들기 -> 필요 없을 듯 하다. 그냥 docker, podman, buildah, 기타 툴로 하면 될 듯.
2. 각각 container, volume, image 등의 이름 테그 기타 정보들을 자동으로 규칙적으로 만들어주는 방안 생각해야함.
3. 컨테이너의 healthcheck, healthcheck 에 따라서 반응하는 루틴 필요 - 우선적으로 처리하기 -> dag-go 연결때문에.
4. 호스트에서 컨테이너로 데이터 전송 podman cp 관련 자료 찾기
5. https://go.dev/doc/articles/race_detector
6. https://eng.uber.com/dynamic-data-race-detection-in-go-code/
7. https://hackernoon.com/how-to-write-benchmarks-in-golang-like-an-expert-0w1834gs

기타 정리 안된 내용

- ipc 로 restful 연결 임으로 지속적으로 연결을 유지할 필요가 없지 않을까?
- 여기서는 client 입장에서 접근 한다.
- https://github.com/james-barrow/golang-ipc 참고
- 에러에 관해서 좀 살펴보자.
  참고 : http://cloudrain21.com/golang-graceful-error-handling
- podman/libpod 에서 container.go 잘 살펴보기

## 읽어보기
https://medium.com/safetycultureengineering/an-overview-of-memory-management-in-go-9a72ec7c76a8
