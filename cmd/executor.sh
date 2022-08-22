#!/bin/bash

# 여기에 사용자의 shell script 가 들어가고 이이 파일을 구동시킨다.

# subshell 에서 exit 가능하도록: set -o errexit
# 파이프 명령 라인 모두에서 오류 종료 상태 값 인식: set -o pipefail
# 여기서 종료 코드를 log 에 기록한다.
set -o pipefail -o errexit

# 자신의 pid($$) 를 log 에 기록한다.
echo "pid:"$$ | tee ./log

# 내일 자바로 테스트 해보자.
# 사용자 작성 코드가 들어오는 위치
fallocate -l 10G test10g.txt
cp test10g.txt ../shell

# exit code 를 log 에 기록한다.
echo "exit:"$? | tee ./log


#[ $status -eq 0 ] && echo "command successful" || echo "command unsuccessful >&2"
#if [ $status -eq 0 ]
#then
#  echo "command successful"
#else
#  echo "command unsuccessful >&2"
#fi

