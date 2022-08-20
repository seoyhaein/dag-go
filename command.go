package dag_go

import "fmt"

// interface 로 만들자.
type Command struct {
	// TODO 입력 파라미터를 넣을지는 고민하고 일단 지운다.
	//RunE func(args []string) error
	this *Node
	RunE func(n *Node) error
}

func (c *Command) Execute() (err error) {
	if c.this == nil {
		return fmt.Errorf("node's pointer is not set")
	}
	err = c.execute(c.this)
	return
}

func (c *Command) execute(this *Node) (err error) {
	err = c.RunE(this)
	return
}

func CreateCommand(n *Node, r Runnable) *Command {
	if n == nil {
		return nil
	}
	cmd := &Command{
		this: n,
	}
	cmd.RunE = func(n *Node) error {

		return r.RunE(n)
	}
	return cmd
}

/*func (c *Command) Set(f func() error) {

	g := func(n *Node) error {
		f()
		return nil
	}
	c.RunE = g
}*/

type Runnable interface {
	RunE(*Node) error
}
