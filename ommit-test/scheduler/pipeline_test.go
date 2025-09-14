package scheduler

import (
	"strings"
	"testing"
)

// TestParsePipeline_JSONC_SetsIDs tests that ParsePipeline correctly parses a JSONC input and sets node IDs.
// 테스트 코드에 다소 오류가 있지만 넘어간다.
func TestParsePipeline_JSONC_SetsIDs(t *testing.T) {
	input := `{
  // top-level comment
  "nodes": {
    "start": {
      "type": "start",
      "ui": { "position": { "x": 400, "y": 50 } },
      "graph": { "dependsOn": [] },
      "storage": {
        "host": { "upperDir": null, "workDir": null, "mergedDir": null, "lowerDirs": [] },
        "containerPath": null
      },
      "container": {
        "image": "alpine:latest",
        "env": { "runId": "AUTO" },
        "resources": {
          "limits": { "cpu": "250m", "memory": "256Mi" },
          "requests": { "cpu": "250m", "memory": "256Mi" }
        },
        "userScript": "#!/bin/sh\nset -eu\necho start\n",
        "mounts": [
          { "type": "bind", "hostPath": "/mnt/overlay", "containerPath": "/overlay" }
        ]
      }
    },

    "1": {
      "type": "task",
      "ui": { "position": { "x": 400, "y": 180 } },
      "graph": { "dependsOn": ["start"] },
      "storage": {
        "host": {
          "upperDir": "@baseDir/@runId/@nodeId-upper",
          "workDir": "@baseDir/@runId/@nodeId-work",
          "mergedDir": "@baseDir/@id/@runId/merged/@nodeId",
          "lowerDirs": ["@baseDir/lower"]
        },
        "containerPath": "/data/step1"
      },
      "container": {
        "image": "pipeline_imgA:latest",
        "env": {
          "TMPDIR": "/data/step1/.tmp",
          "XDG_CACHE_HOME": "/data/step1/.cache",
          "HOME": "/data/step1"
        },
        "resources": {
          "limits": { "cpu": "500m", "memory": "512Mi" },
          "requests": { "cpu": "500m", "memory": "512Mi" }
        },
        "userScript": "#!/bin/sh\nset -eu\necho node1\n",
        "mounts": [
          { "type": "volume", "name": "ref-fa", "hostPath": "/data/ref.fa", "containerPath": "/mnt/ref.fa", "accessMode": "ro" }
        ]
      }
    },

    "2": {
      "type": "task",
      "ui": { "position": { "x": 100, "y": 280 } },
      "graph": { "dependsOn": ["1"] },
      "storage": {
        "host": {
          "upperDir": "@baseDir/@runId/@nodeId-upper",
          "workDir": "@baseDir/@runId/@nodeId-work",
          "mergedDir": "@baseDir/@id/@runId/merged/@nodeId",
          "lowerDirs": ["@baseDir/@id/@runId/merged/1", "@baseDir/lower"]
        },
        "containerPath": "/data/step2"
      },
      "container": {
        "image": "pipeline_img1:latest",
        "env": {
          "TMPDIR": "/data/step2/.tmp",
          "XDG_CACHE_HOME": "/data/step2/.cache",
          "HOME": "/data/step2"
        },
        "resources": {
          "limits": { "cpu": "500m", "memory": "512Mi" },
          "requests": { "cpu": "500m", "memory": "512Mi" }
        },
        "userScript": "#!/bin/sh\nset -eu\necho node2\n",
        "mounts": [
          { "type": "volume", "name": "ref-fa", "hostPath": "/data/ref.fa", "containerPath": "/mnt/ref.fa", "accessMode": "ro" }
        ]
      }
    },

    "end": {
      "type": "end",
      "ui": { "position": { "x": 400, "y": 480 } },
      "graph": { "dependsOn": ["2"] },
      "storage": {
        "host": { "upperDir": null, "workDir": null, "mergedDir": null, "lowerDirs": [] },
        "containerPath": null
      },
      "container": {
        "image": "alpine:latest",
        "resources": {
          "limits": { "cpu": "250m", "memory": "256Mi" },
          "requests": { "cpu": "250m", "memory": "256Mi" }
        },
        "userScript": "#!/bin/sh\nset -eu\necho end\n",
        "mounts": []
      }
    }
  }
}`

	p, err := ParsePipeline(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParsePipeline failed: %v", err)
	}
	if p == nil {
		t.Fatalf("ParsePipeline returned nil pipeline")
	}
	if p.Nodes == nil {
		t.Fatalf("p.Nodes is nil")
	}

	// 키/ID 확인
	want := []string{"start", "1", "2", "end"}
	for _, id := range want {
		n, ok := p.Nodes[id]
		if !ok {
			t.Fatalf("node %q missing in map", id)
		}
		if n == nil {
			t.Fatalf("node %q is nil", id)
		}
		if n.ID != id {
			t.Fatalf("node %q has ID %q (want %q)", id, n.ID, id)
		}
	}
}

func TestParsePipeline0(t *testing.T) {
	p, err := ParsePipelineFile("../pipeline.jsonc")
	if err != nil {
		t.Fatalf("ParsePipelineFile error: %v", err)
	}
	if p == nil {
		t.Fatal("got nil pipeline")
	}
	if len(p.Nodes) == 0 {
		t.Fatal("pipeline has no nodes")
	}

	// 노드 ID/의존성 정합성 확인 (NormalizeDepends 결과 기준)
	for id, n := range p.Nodes {
		if n == nil {
			t.Fatalf("node %q is nil", id)
		}
		if n.ID != id {
			t.Fatalf("node %q has mismatched ID field: got %q", id, n.ID)
		}

		prev := ""
		seen := make(map[string]struct{}, len(n.Graph.DependsOn))
		for i, dep := range n.Graph.DependsOn {
			if dep == "" {
				t.Fatalf("node %q has empty dependsOn at index %d", id, i)
			}
			if dep == id {
				t.Fatalf("node %q depends on itself", id)
			}
			if _, ok := p.Nodes[dep]; !ok {
				t.Fatalf("node %q depends on unknown node %q", id, dep)
			}
			if _, dup := seen[dep]; dup {
				t.Fatalf("node %q dependsOn contains duplicate %q", id, dep)
			}
			seen[dep] = struct{}{}

			// NormalizeDepends 에서 sort.Strings(out)로 정렬했는지 체크
			if i > 0 && prev > dep {
				t.Fatalf("node %q dependsOn not sorted: %q before %q", id, prev, dep)
			}
			prev = dep
		}
	}
}
