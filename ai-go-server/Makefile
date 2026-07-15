.PHONY: run minicall docqa test fmt vet

run:
	go run .

minicall:
	go run ./cmd/minicall -- $(QUESTION)

docqa:
	go run ./cmd/docqa -- -docs "$(DOCS)" "$(QUESTION)"

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

vet:
	go vet ./...
