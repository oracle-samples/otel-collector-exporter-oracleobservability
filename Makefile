.PHONY: fmt test vet tidy

fmt:
	gofmt -w $$(find oracleobservabilityexporter -name '*.go')

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy
