.DEFAULT_GOAL := help
.PHONY: help check test vet fmt lint cover fuzz bench

help: ## show this list
	@grep -hE '^[a-z][a-z0-9-]*:.*##' $(MAKEFILE_LIST) | sed -e 's/:.*##/|/' | column -t -s '|'

check: fmt vet lint test ## everything CI runs, in the same order

test: ## tests, with the same flags CI uses plus a profile for `make cover`
	go test -race -shuffle=on -coverprofile=coverage.out ./...

vet: ## go vet
	go vet ./...

fmt: ## fail if anything is not gofmt'd
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt'd:"; echo "$$unformatted"; exit 1; \
	fi

lint: ## golangci-lint
	golangci-lint run ./...

cover: test ## open the coverage report
	go tool cover -html=coverage.out

fuzz: ## short fuzz smoke over the parser, as CI runs it
	go test -run Fuzz -fuzz FuzzParse -fuzztime 30s
	go test -run Fuzz -fuzz 'FuzzRoundTrip$$' -fuzztime 30s
	go test -run Fuzz -fuzz 'FuzzModelSurvivesEncoding$$' -fuzztime 30s
	go test -run Fuzz -fuzz 'FuzzCheck$$' -fuzztime 30s
	go test -run Fuzz -fuzz 'FuzzRegroup$$' -fuzztime 30s

bench: ## benchmarks; compare runs with benchstat
	go test -run '^$$' -bench . -benchmem -count 8
