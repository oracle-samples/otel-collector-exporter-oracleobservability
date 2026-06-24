.PHONY: fmt test vet tidy

fmt:
	gofmt -w $$(find oracleobservabilityexporter -name '*.go')

test:
	go -C oracleobservabilityexporter test ./...

vet:
	go -C oracleobservabilityexporter vet ./...

tidy:
	go -C oracleobservabilityexporter mod tidy
