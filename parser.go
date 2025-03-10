package dag_go

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
)

// TODO xml 에서 nodes id 가 있어야 하고, pipeline id 와 매핑 되고,
// dag ID 는 uuid 인데, 이것은 xml 에서 데이터와 연계 될때 생성되는 uuid 값을 가지고 와서 매핑 된다.
// TODO xmlNode 를 별도로 두었지만, Node 로 통합하자.
// Nodes 를 어디에 둘지 고민..
// xml 이 수정될때 마다 수정해야 함으로 어느정도 완성된다음에 코드 수정 및 정리를 하자.

func xmlParser(x []*Node) (context.Context, bool, *Dag) {

	if x == nil {
		return nil, false, nil
	}

	n := len(x)
	//TODO 일단 에러 때문에 이렇게 넣었지만, 이걸 외부로 빼야함.
	//runnable := Connect()
	dag := NewDag()

	if n >= 1 { // node 가 최소 하나 이상은 있어야 한다.
		//dag := NewDag()
		// 순서데로 들어가기 때문에 for range 보다 유리함.
		for i := 0; i < n; i++ {
			no := x[i]

			// from 이 없으면 root 임.
			rn := len(no.from)
			if rn == 0 {
				dag.AddNodeToStartNode(no)
			}
			// 자신이 from 이므로, to 만 신경쓰면 된다.
			for _, v := range no.to {
				from := findNode(x, v)
				dag.AddEdge(no.Id, from.Id)
			}
		}

		//visited := dag.visitReset()
		//dag.detectCycle(dag.StartNode.Id, dag.StartNode.Id, visited)
		//result, _ := dag.detectCycle(dag.startNode.Id, dag.startNode.Id, visited)
		//fmt.Printf("%t", result)

		// 테스트 용도로 일단 넣어둠.
		dag.FinishDag()

		ctx := context.Background()
		dag.ConnectRunner()
		//dag.DagSetFunc()
		dag.GetReady(ctx)
		//dag.start()

		//b := dag.Wait(nil)
		//fmt.Println("모든 고루틴이 종료될때까지 그냥 기다림.")
		//fmt.Printf("true 이면 정상 : %t", b)

		return ctx, true, dag
	}

	return nil, false, nil
}

// TODO 함수들 정리해서 놓자.

func xmlProcess(parser *xml.Decoder) (int, []*Node) {
	var (
		counter         = 0
		n       *Node   = nil
		ns      []*Node = nil
		// TODO bool 로 하면 안됨 int 로 바꿔야 함. 초기 값은 0, false = 1, true = 2 로
		xStart    = false
		nStart    = false
		cmdStart  = false
		fromStart = false
		toStart   = false
	)
	// TODO error 처리 해줘야 함.
	if parser == nil {
		return 0, nil
	}

	for {
		token, err := parser.Token()

		// TODO 아래 두 if 구문 향후 수정하자.
		if err == io.EOF {
			break // TODO break 구문 수정 처리 필요.
		}

		// TODO 중복되는 것 같지만 일단 그냥 넣어둠.
		if token == nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			//elem := xml.StartElement(t)
			xmlTag := t.Name.Local
			if xmlTag == nodes {
				xStart = true
			}
			if xmlTag == node {
				nStart = true

				n = new(Node)
				// node id 가 없으면 error 임.
				// 현재는 node 의 경우 속성이 1 이도록 강제함. 추후 수정할 필요가 있으면 수정.
				if len(t.Attr) != 1 {
					fmt.Println("node id 가 없음")
					return 0, nil
				}

				// TODO id 가 아니면 error	- strict 하지만 일단 이렇게
				if t.Attr[0].Name.Local == id {
					for _, v := range ns {
						if v.Id == t.Attr[0].Value {
							fmt.Println("중복된 node Id 존재")
							return 0, nil
						}
					}
					n.Id = t.Attr[0].Value
				}
			}

			if xmlTag == command {
				cmdStart = true
			}

			if xmlTag == from {
				fromStart = true
			}

			if xmlTag == to {
				toStart = true
			}

			counter++
		case xml.EndElement:
			//elem := xml.EndElement(t)
			xmlTag := t.Name.Local
			if xmlTag == nodes {
				if xStart {
					xStart = false
				} else {
					fmt.Println("error") // TODO error 처리 해줘야 함.
				}
			}
			// TODO 중복 구문들 function 으로 만들자.
			if xmlTag == node {
				if xStart { // StartElement 에서 true 해줌
					if nStart { // StartElement 에서 true 해줌
						if n != nil { // TODO nil 일 경우는 에러 처리 해줘야 함.
							ns = append(ns, n)
							nStart = false
							n = nil
						}
					}
				}
			}

			if xmlTag == from {
				if xStart {
					if nStart {
						if n != nil {
							fromStart = false
						}
					}
				}
			}

			if xmlTag == to {
				if xStart {
					if nStart {
						if n != nil {
							toStart = false
						}
					}
				}
			}

			if xmlTag == command {
				if xStart {
					if nStart {
						if cmdStart {
							cmdStart = false
						}
					}
				}
			}
			counter++

		case xml.CharData:
			if nStart {
				if n != nil {
					if cmdStart {
						// TODO string converting 바꾸기
						n.Commands = string(t)
					}
					if fromStart {
						n.from = append(n.from, string(t))
					}
					if toStart {
						n.to = append(n.to, string(t))
					}
				}
			}
		}
	}

	// TODO  일단 이렇게 그냥 해둠.
	return counter, ns
}

func newDecoder(b []byte) *xml.Decoder {
	// (do not erase) NewDecoder 에서 Strict field true 해줌.
	d := xml.NewDecoder(bytes.NewReader(b))
	return d
}

func XmlParser(d []byte) (context.Context, bool, *Dag) {

	decoder := newDecoder(d)
	_, nodes := xmlProcess(decoder)
	ctx, b, dag := xmlParser(nodes)

	return ctx, b, dag
}
