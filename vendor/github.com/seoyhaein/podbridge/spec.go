package podbridge

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/containers/podman/v4/pkg/domain/entities"
	deepcopy "github.com/containers/podman/v4/pkg/domain/utils"
	"github.com/containers/podman/v4/pkg/specgen"
)

// 대문자로 할지 고민해보자.
type pair struct {
	p1 any
	p2 any
}

func MakePair(a any, b any) *pair {
	return &pair{
		p1: a,
		p2: b,
	}
}

// 이렇게 만드는 것 고민해보자. 많이 사용하는 field에 대해서
func Name(name string) *pair {
	return &pair{
		p1: "Name",
		p2: name,
	}
}

var (
	Spec   *specgen.SpecGenerator
	backup *specgen.SpecGenerator

	PodSpec   *entities.PodSpec
	podbackup *entities.PodSpec

	cleanBackup bool
)

type (
	Option    func(*specgen.SpecGenerator) Option
	PodOption func(*entities.PodSpec) PodOption
)

// default 값을 세팅할 수 있다.
// TODO PodbridgeInit -> basket.go 참고 이걸로 통일하는 방향으로 가자.
func init() {
	Spec = new(specgen.SpecGenerator)
	backup = new(specgen.SpecGenerator)

	PodSpec = new(entities.PodSpec)
	podbackup = new(entities.PodSpec)
	cleanBackup = true
}

func eraseSpec(spec *specgen.SpecGenerator) *specgen.SpecGenerator {
	clear := clearStruct(spec)

	if clear == nil {
		return nil
	}
	s, b := clear.(*specgen.SpecGenerator)

	if b {
		return s
	} else {
		return nil
	}

}

func erasePodSpec(podspec *entities.PodSpec) *entities.PodSpec {
	clear := clearStruct(podspec)

	if clear == nil {
		return nil
	}
	s, b := clear.(*entities.PodSpec)

	if b {
		return s
	} else {
		return nil
	}

}

// TODO test
/*
	If a reflect.Value is a pointer, then v.Elem() is equivalent to reflect.Indirect(v). If it is not a pointer, then they are not equivalent:
		If the value is an interface then reflect.Indirect(v) will return the same value, while v.Elem() will return the contained dynamic value.
		If the value is something else, then v.Elem() will panic.
	The reflect.Indirect helper is intended for cases where you want to accept either a particular type, or a pointer to that type.
	One example is the database/sql conversion routines: by using reflect.Indirect, it can use the same code paths to handle the various types and pointers to those types.
*/

func clearStruct(a interface{}) interface{} {

	v := reflect.Indirect(reflect.ValueOf(a))
	if v.Kind() == reflect.Struct {
		v.Set(reflect.Zero(v.Type()))

		return a
	}

	return nil
}

// TODO test
// TODO 읽기 https://betterprogramming.pub/implementing-type-safe-tuples-with-go-1-18-9624010efaa
// TODO 이런 함수의 형태는 error 처리가 좀 힘든데, 어떻할지 생각해보자. 일단 panic 으로 설정함.
// https://github.com/golang/example/tree/master/gotypes
// 1.18 에서는 constraints: move to x/exp for Go 1.18 이렇게됨. 향후 조정될 수도 있음.
// https://github.com/golang/go/issues/50792
// 참고 : https://pkg.go.dev/go/types
// tuple 은 사용하지 않는다.
// a 는 컨테이너나 pod 의 spec struct 이고, p 는 struct 에 넣을 필드와 값이다.
// 일단 string 만 적용되도록 했다.
// spec 과 Spec 이름을 혼동하지 말자.
// 여기서 struct 의 field name 을 잘못 작성하면 panic 이 발생하도록 했다.
// TODO 반드시 Set 해야 하는 field 에 대한 부분을 넣어줘야 한다.

func WithValues(as ...any) Option {
	return func(spec *specgen.SpecGenerator) Option {

		// for backup
		if spec == nil {
			for _, a := range as {
				s, b := a.(*specgen.SpecGenerator)
				// 여기서 s 는 backup 이다.
				if b {
					Spec = eraseSpec(Spec)
					// TODO error 처리하자.
					deepcopy.DeepCopy(Spec, s)
					cleanBackup = false
				}
			}
		} else {
			if cleanBackup == false {
				backup = eraseSpec(backup)
				cleanBackup = true
			}
			deepcopy.DeepCopy(backup, Spec)

			for _, a := range as {
				p, b := a.(*pair)
				if b {
					v1, b1 := p.p1.(string)
					if b1 {
						err := SetField(spec, v1, p.p2)
						if err != nil {
							panic(err.Error())
						}
					} else {
						panic(errors.New("the type of field name must be string"))
					}
				}
			}
		}
		return WithValues(backup)
	}
}

func SetField(a interface{}, fieldName string, value interface{}) error {
	v := reflect.Indirect(reflect.ValueOf(a))
	if v.CanAddr() == false {
		return fmt.Errorf("cannot assign to the item passed, item must be a pointer in order to assign")
	}

	fieldVal := v.FieldByName(fieldName)
	if fieldVal.CanSet() {
		fieldVal.Set(reflect.ValueOf(value))
	}

	return nil
}

func Finally(option ...Option) {

	for _, op := range option {
		op(nil)
	}
}

func FinallyPod(podoption ...PodOption) {

	for _, op := range podoption {
		op(nil)
	}
}

func WithPodValues(as ...any) PodOption {
	return func(podspec *entities.PodSpec) PodOption {

		// for backup
		if podspec == nil {
			for _, a := range as {
				s, b := a.(*entities.PodSpec)
				// 여기서 s 는 backup 이다.
				if b {
					PodSpec = erasePodSpec(PodSpec)
					deepcopy.DeepCopy(PodSpec, s)
					cleanBackup = false
				}
			}
		} else {
			if cleanBackup == false {
				podbackup = erasePodSpec(podbackup)
				cleanBackup = true
			}
			deepcopy.DeepCopy(podbackup, PodSpec)

			for _, a := range as {
				p, b := a.(*pair)
				if b {
					v1, b1 := p.p1.(string)
					if b1 {
						err := SetField(podspec, v1, p.p2)
						if err != nil {
							panic(err.Error())
						}
					} else {
						panic(errors.New("the type of field name must be string"))
					}
				}
			}
		}
		return WithPodValues(podbackup)
	}
}
