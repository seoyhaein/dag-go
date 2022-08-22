#!/usr/bin/env bash

# alphine 에서는 bash 가 없으므로 설치해줘야 한다.
# 특정 shell script의 pid 를 확인하여, 진행사항을 파악하고 이 내용들을 환경 변수에 등록한다.

# https://codechacha.com/ko/shell-script-read-file/
# https://codechacha.com/ko/shell-script-substring/
# https://codechacha.com/ko/shell-script-compare-string/
# https://www.lesstif.com/lpt/linux-tee-89556049.html

#첫 번째 필드
#D io와 같이 중지(interrupt)시킬 수 없는 잠자고 있는 (휴지) 프로세스 상태
#R 현제 동작중이거나 동작할 수 있는 상태
#S 잠자고 있지만, 중지시킬수 있는 상태
#T 작업 제어 시그널로 정지되었거나 추적중에 있는 프로세스 상태
#X 완전히 죽어 있는 프로세tm
#Z 죽어 있는 좀비 프로세스

#두 번째 필드
#< 프로세스의 우선 순위가 높은 상태
#N 프로세스의 우선 순위가 낮은 상태
#L 실시간이나 기존 IO를 위해 메모리 안에 잠겨진 페이지를 가진 상태
#s 세션 리터(주도 프로세스)
#I 멀티 쓰레드
#+ 포어그라운드 상태로 동작하는 프로세스

i=1
while read line || [ -n "$line" ] ; do
  if [[ "$line" == *pid* ]]; then
    pid=${line#pid:}
    echo "$pid"
    status=$(ps -f "$pid" | awk '{print $7}')
    status=${status#STAT}
    #status=${status##+( )}
    echo "$status"
    # 추가 조건식이 들어가야함.
  fi
# https://mongdols.tistory.com/18 , shell script 조건식
  if [[ "$line" == *exit* ]]; then
      exitCode=${line#exit:}
      if [ "$exitCode" -ne 0 ]
      then
        echo "$exitCode"
        exit 1
      else
        echo "$exitCode"
        exit 0
      fi
  fi
 # echo "Line $i: $line"
  ((i+=1))
done < log













