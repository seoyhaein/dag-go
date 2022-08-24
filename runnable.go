package dag_go

type Runnable interface {
	RunE(a interface{}) (int, error)
	CreateImage(a interface{}, healthChecker string) error
	Execute() (r int, err error)
}
