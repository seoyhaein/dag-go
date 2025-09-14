package scheduler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// template pipeline 에서 데이터만 붙으면 런타임 파이프라인으로 갈 수 있어야 함.
// 파이프라인 정보를 담든 것부터 시작한다.
// 이 런타임 파이프라인은 dag 로 이제 변경되어야 함. -> 이건 executor 에서

// Pipeline is the top-level document.
type Pipeline struct {
	// TODO 이거 이름 바꾸던가 하자. json 에서 일단 이름 바꾸는 것을 한번 고려해보자.
	Nodes map[string]*Node `json:"nodes"`
}

// Node is a single vertex in the DAG.
type Node struct {
	ID   string `json:"-"`    // derived from key in Nodes
	Type string `json:"type"` // "start" | "task" | "end" (extensible)
	//UI UINode `json:"ui"`
	Graph GraphSpec `json:"graph"`
	//Storage StorageSpec `json:"storage"`
	//Container ContainerSpec `json:"container"`
}

type GraphSpec struct {
	DependsOn []string            `json:"dependsOn"`
	depSet    map[string]struct{} // 내부용
}

// ParsePipeline json 파싱.
// TODO 생각하기 중요. 템플릿 파이프라인을 만들어 주는데, 일단 런타임 파이프라인은 다르게 처리해주어야 함.
func ParsePipeline(r io.Reader) (*Pipeline, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	// TODO 향후 주석 처리. jsonc 처리 하지만, json 으로만 처리할 예정임.
	data = stripJSONComments(data)
	var p Pipeline
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	// Fill Node IDs from map keys
	for id, n := range p.Nodes {
		if n != nil {
			n.ID = id
		}
	}

	if err := NormalizeDepends(&p); err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	return &p, nil
}

// ParsePipelineFile opens a file and parses it via ParsePipeline.
func ParsePipelineFile(path string) (*Pipeline, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			// TODO 일단 이렇게 처리함. 이후 바꾸어야 함.
			fmt.Printf("error closing file: %v\n", err)
		}
	}(f)
	return ParsePipeline(f)
}

// stripJSONComments removes // line comments and /* ... */ block comments while
// preserving content inside JSON strings. It does not implement full JSON5;
// trailing commas are still invalid. This is sufficient for typical JSONC.
func stripJSONComments(in []byte) []byte {
	var out bytes.Buffer
	inStr := false
	inSL := false // single-line comment
	inML := false // multi-line comment
	esc := false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inStr {
			out.WriteByte(c)
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		if inSL {
			// consume until newline
			if c == '\n' || c == '\r' {
				inSL = false
				out.WriteByte(c)
			}
			continue
		}
		if inML {
			if c == '*' && i+1 < len(in) && in[i+1] == '/' {
				inML = false
				i++ // skip '/'
			}
			continue
		}
		if c == '"' {
			inStr = true
			out.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(in) {
			n := in[i+1]
			if n == '/' {
				inSL = true
				i++
				continue
			}
			if n == '*' {
				inML = true
				i++
				continue
			}
		}
		out.WriteByte(c)
	}
	return out.Bytes()
}

func NormalizeDepends(p *Pipeline) error {
	if p == nil || len(p.Nodes) == 0 {
		return errors.New("invalid pipeline")
	}
	for id, n := range p.Nodes {
		if n == nil {
			return fmt.Errorf("node %q is nil", id)
		}
		deps := n.Graph.DependsOn
		seen := make(map[string]struct{}, len(deps))
		out := deps[:0] // 백킹 배열 재사용

		for _, dep := range deps {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if dep == id {
				return fmt.Errorf("node %q depends on itself", id)
			}
			if _, ok := p.Nodes[dep]; !ok {
				return fmt.Errorf("node %q depends on unknown %q", id, dep)
			}
			if _, dup := seen[dep]; dup {
				continue
			}
			seen[dep] = struct{}{}
			out = append(out, dep)
		}

		// 결정론: 항상 같은 순서로 고정 (테스트/해시 안정)
		sort.Strings(out)

		// 정규화 결과 반영 + 내부 캐시
		n.Graph.DependsOn = out
		n.Graph.depSet = seen // 추후 확장성을 위해서 일단 남겨둠.
	}
	return nil
}
