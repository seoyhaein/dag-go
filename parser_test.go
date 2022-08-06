package dag_go

import (
	"fmt"
	"testing"
)

func TestXmlProcess(t *testing.T) {
	d := serveXml()
	decoder := newDecoder(d)
	c, nodes := xmlProcess(decoder)

	fmt.Println("count :", c)

	for _, node := range nodes {
		fmt.Println("Node Id: ", node.Id)
		for _, t := range node.to {
			fmt.Println("To", t)
		}
		for _, f := range node.from {
			fmt.Println("From", f)
		}
		fmt.Println("Command ", node.commands)

	}
}

func TestXmlsProcess(t *testing.T) {
	xmls := serveXmls()
	num := len(xmls)
	if num == 0 {
		fmt.Println("값이 없음")
		return
	}

	for _, xml := range xmls {
		d := []byte(xml)

		decoder := newDecoder(d)
		c, nodes := xmlProcess(decoder)

		fmt.Println("count :", c)

		for _, node := range nodes {
			fmt.Println("Node Id: ", node.Id)
			for _, t := range node.to {
				fmt.Println("To", t)
			}
			for _, f := range node.from {
				fmt.Println("From", f)
			}
			fmt.Println("Command ", node.commands)

		}
	}

}

// 여러 형태의 xml 테스트 필요, 깨진 xml 로도 테스트 진행 필요.
// TODO nodes id 를 고유하게 만들어줘서, 이걸 dag id 로 넣어주자.
func serveXml() []byte {
	// id, to, from, command
	// id 는 attribute, to, from, command 는 tag
	// to, from 은 복수 가능.
	// from 이 없으면 시작노드, 파싱된 후에 start_node, end_node 추가 됨.
	xml := `
	<nodes>
		<node id = "1">
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<to>8</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<to>4</to>
			<to>5</to>
			<command>echo "hello world 3"</command>
		</node>
		<node id ="4" >
			<from>3</from>
			<to>6</to>
			<command>echo "hello world 4"</command>
		</node>
		<node id ="5" >
			<from>3</from>
			<to>6</to>
			<command>echo "hello world 5"</command>
		</node>
		<node id ="6" >
			<from>4</from>
			<from>5</from>
			<to>7</to>
			<command>echo "hello world 6"</command>
		</node>
		<node id ="7" >
			<from>6</from>
			<to>9</to>
			<to>10</to>
			<command>echo "hello world 7"</command>
		</node>
		<node id ="8" >
			<from>2</from>
			<command>echo "hello world 8"</command>
		</node>
		<node id ="9" >
			<from>7</from>
			<to>11</to>
			<command>echo "hello world 9"</command>
		</node>
		<node id ="10" >
			<from>7</from>
			<to>11</to>
			<command>echo "hello world 10"</command>
		</node>
		<node id ="11" >
			<from>9</from>
			<from>10</from>
			<command>echo "hello world 11"</command>
		</node>
	</nodes>`

	return []byte(xml)
}

func serveXmls() []string {
	var xs []string

	// nodes 가 없는 경우
	failedXml3 := `
		<node id = "1" >
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>`

	//xs = append(xs, failedXml1)
	//xs = append(xs, failedXml2)
	xs = append(xs, failedXml3)
	//xs = append(xs, failedXml4)

	return xs
}

func xmlss() {
	// id 가 없는 node
	failedXml1 := `
	<nodes>
		<node>
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>
	</nodes>`

	// node id 가 중복된 경우
	failedXml2 := `
	<nodes>
		<node id = "2" >
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>
	</nodes>`

	// nodes 가 없는 경우
	failedXml3 := `
		<node id = "1" >
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>`

	// circle TODO 다수 테스트 진행해야함.
	failedXml4 := `
	<nodes>
		<node id = "1">
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<to>1</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<command>echo "hello world 3"</command>
		</node>
	</nodes>`

	fmt.Println(failedXml1)
	fmt.Println(failedXml2)
	fmt.Println(failedXml3)
	fmt.Println(failedXml4)
}

// parser 테스트
// TODO 버그 있음. read |0: file already closed
// xmlParser 에서 입력 파라미터 포인터로 할지 생각하자.
// 순서대로 출력되는지 파악해야 한다. 잘못 출력되는 경우 발견.
func TestXmlParser(t *testing.T) {
	d := serveXml()
	decoder := newDecoder(d)
	_, nodes := xmlProcess(decoder)

	ctx, b, dag := xmlParser(nodes)

	if b {
		dag.Start()
		dag.WaitTilOver(ctx)
	}

}
