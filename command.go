package dag_go

type Command struct {
	// TODO 입력 파라미터를 넣을지는 고민하고 일단 지운다.
	//RunE func(args []string) error
	RunE func() error
}

func (c *Command) Execute() (err error) {
	err = c.execute()
	return
}

func (c *Command) execute() (err error) {
	err = c.RunE()
	return
}

// podbridge 를 이용한 apis
