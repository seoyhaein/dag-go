.PHONY: test lint

test:
	go test -v -race -cover ./...

lint:
	golangci-lint run

fmt:
	go fmt ./...