.PHONY: test vet lint sentrux check

test:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

sentrux:
	sentrux check .

check: test vet lint sentrux
