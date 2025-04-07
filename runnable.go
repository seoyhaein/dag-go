package dag_go

type Runnable interface {
	RunE(a interface{}) error
}
