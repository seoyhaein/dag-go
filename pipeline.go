package dag_go

// 생성된 dag 에 데이터를 적용시켜서 pipeline 을 만들어 준다.
// 이때 데이터는 복수일 수 있기 때문에 하나의 dag 에 여러개의 파이프라인이 적용될 수 있다.
// TODO dag id 가 일단 숫자인지 string 인지 생각해야함.

type Pipeline struct {
	id string // dag id + 숫자???
}

// 파이프라인은 dag 와 데이터를 연계해야 하는데 데이터의 경우는 다른 xml 처리하는 것이 바람직할 것이다.
// 외부에서 데이터를 가지고 올 경우, ftp 나 scp 나 기타 다른 프롤토콜을 사용할 경우도 생각을 해야 한다.
