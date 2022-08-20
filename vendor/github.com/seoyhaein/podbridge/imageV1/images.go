package imageV1

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/containers/buildah"
	"github.com/containers/buildah/define"
	is "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/unshare"
	"github.com/opencontainers/go-digest"
	pbr "github.com/seoyhaein/podbridge"
	"github.com/seoyhaein/utils"
)

// https://acet.pe.kr/935

// image id 확인
// podman images --no-trunc

// 사용중인 container 가 검색이 안될때
// podman ps --all --storage

// 모두 지우기
// podman rmi --force -a

var (
	digester = digest.Canonical.Digester()
	log      = pbr.Log

	//TODO 추후 수정 일단 넣어 놓음
	Verbose = true
	Debug   = true
)

type BuilderOption struct {
	BuilderOpt *buildah.BuilderOptions
}

type Builder struct {
	store   storage.Store
	builder *buildah.Builder
}

func NewOption() *BuilderOption {
	opt := new(buildah.BuilderOptions)
	return &BuilderOption{
		BuilderOpt: opt,
	}
}

func (o *BuilderOption) Arg(k string, v string) *BuilderOption {
	// map 중복 체크
	if _, ok := o.BuilderOpt.Args[k]; ok {
		return nil
	}
	o.BuilderOpt.Args[k] = v
	return o
}

func (o *BuilderOption) FromImage(fromImage string) *BuilderOption {
	if utils.IsEmptyString(fromImage) {
		return nil
	}
	o.BuilderOpt.FromImage = fromImage
	return o
}

func (o *BuilderOption) Other() *BuilderOption {
	buildOpts := &buildah.CommonBuildOptions{}
	builderOption := &buildah.BuilderOptions{
		Isolation:        define.IsolationChroot,
		CommonBuildOpts:  buildOpts,
		ConfigureNetwork: buildah.NetworkDefault,
		SystemContext:    &types.SystemContext{},
		Format:           buildah.Dockerv2ImageManifest,
	}

	o.BuilderOpt = builderOption
	return o
}

//TODO context pointer 쓸지 생각하자.

func NewBuilder(ctx context.Context, o *BuilderOption /*opt *buildah.BuilderOptions*/) (context.Context, *Builder, error) {

	store, err := NewStore()
	if err != nil {
		return nil, nil, err
	}
	if o == nil {
		return nil, nil, fmt.Errorf("option is nil")
	}
	opt := o.BuilderOpt
	builder, err := buildah.NewBuilder(ctx, store, *opt)
	if err != nil {
		return nil, nil, err
	}

	b := &Builder{
		store:   store,
		builder: builder,
	}
	return ctx, b, nil
}

func NewStore() (storage.Store, error) {
	buildStoreOptions, err := storage.DefaultStoreOptions(unshare.IsRootless(), unshare.GetRootlessUID())
	if err != nil {
		return nil, err
	}

	buildStore, err := storage.GetStore(buildStoreOptions)
	if err != nil {
		return nil, err
	}

	return buildStore, nil
}

func (b *Builder) Add(from, to string) error {
	err := b.builder.Add(to, false, buildah.AddAndCopyOptions{Hasher: digester.Hash()}, from)
	if err != nil {
		return fmt.Errorf("error while adding: %v", err)
	}
	return nil
}

// TODO > or >> 등 파이프 관련해서 작동하지 않음.

func (b *Builder) Run(s string) error {

	logger := GetLoggerWriter()
	runOptions := buildah.RunOptions{
		Stdout:    logger,
		Stderr:    logger,
		Isolation: define.IsolationChroot,
	}
	var (
		ac [][]string
		c  []string
	)
	command := strings.Split(s, " ")
	for i := 0; i < len(command); i++ {
		if command[i] == "&&" {
			ac = append(ac, c)
			c = nil
		} else {
			c = append(c, command[i])
		}
	}
	if len(c) > 0 {
		ac = append(ac, c)
	}
	for j := 0; j < len(ac); j++ {
		err := b.builder.Run(ac[j], runOptions)
		if err != nil {
			return fmt.Errorf("error while runnning command: %v", err)
		}
	}
	return nil
}

/*func (b *Builder) RunA(s string) error {

	logger := GetLoggerWriter()
	runOptions := buildah.RunOptions{
		Stdout:    logger,
		Stderr:    logger,
		Isolation: define.IsolationChroot,
	}

	command := strings.Split(s, " ")

	err := b.builder.Run(command, runOptions)
	if err != nil {
		return fmt.Errorf("error while runnning command: %v", err)
	}
	return nil
}*/

func (b *Builder) WorkDir(path string) error {
	if utils.IsEmptyString(path) {
		return fmt.Errorf("path is empty")
	}
	b.builder.SetWorkDir(path)
	return nil
}

func (b *Builder) Env(k, v string) error {
	if utils.IsEmptyString(k) || utils.IsEmptyString(v) {
		return fmt.Errorf("key or valeu is empty")
	}

	b.builder.SetEnv(k, v)
	return nil
}

func (b *Builder) User(u string) error {
	if utils.IsEmptyString(u) {
		return fmt.Errorf("user is empty")
	}

	b.builder.SetUser(u)
	return nil
}

func (b *Builder) Expose(port string) error {
	if utils.IsEmptyString(port) {
		return fmt.Errorf("port is empty")
	}
	b.builder.SetPort(port)
	return nil
}

func (b *Builder) Cmd(cmd ...string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("command is empty")
	}
	b.builder.SetCmd(cmd)
	return nil
}

func (b *Builder) CommitImage(ctx context.Context, preferredManifestType string, sysCtx *types.SystemContext, repository string) (*string, error) {

	imageRef, err := is.Transport.ParseStoreReference(b.store, repository)
	if err != nil {
		return nil, err
	}

	imageId, _, _, err := b.builder.Commit(ctx, imageRef, buildah.CommitOptions{
		PreferredManifestType: preferredManifestType,
		SystemContext:         sysCtx,
	})
	if pbr.Basket != nil {
		pbr.Basket.AddImagesId(imageId)
	}
	return &imageId, err
}

func (b *Builder) Delete() error {
	err := b.builder.Delete()

	if err != nil {
		return err
	}
	return nil
}

func (b *Builder) Shutdown() error {
	_, err := b.store.Shutdown(false)

	if err != nil {
		return err
	}
	return nil
}

func GetLoggerWriter() io.Writer {
	if Verbose || Debug {
		return os.Stdout
	} else {
		return NopLogger{}
	}
}

type NopLogger struct{}

func (n NopLogger) Write(p []byte) (int, error) {
	return len(p), nil
}

// TODO buildah/config.go 참고
// 만들어진 이미지 검사
// https://stackoverflow.com/questions/19104847/how-to-generate-a-dockerfile-from-an-image
// https://github.com/containers/buildah/blob/main/troubleshooting.md verify
