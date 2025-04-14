.PHONY: test lint

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	go fmt ./...