.PHONY: run minicall test fmt vet

run:
	go run .

minicall:
	go run ./cmd/minicall -- $(QUESTION)

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

vet:
	go vet ./...
