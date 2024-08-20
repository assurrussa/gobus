.DEFAULT_GOAL := check
UNIT_COVERAGE_MIN := 90

check: fmt vet lint test test-race cover-html cover

fmt:
	go fmt ./...

lint:
	golangci-lint run -v --fix --timeout=5m ./...

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race -count=5 ./...

cover-html:
	@go test -coverprofile=./coverage.text -covermode=atomic $(shell go list ./...)
	@go tool cover -html=./coverage.text -o ./cover.html && rm ./coverage.text

cover:
	@go test -coverpkg=./... -coverprofile=./cover_profile.out.tmp $(go list ./...)
	@grep -v -e "mock" -e "\.pb\.go" -e "\.pb\.validate\.go" ./cover_profile.out.tmp > ./cover_profile.out && rm ./cover_profile.out.tmp
	@CUR_COVERAGE=$(shell go tool cover -func=cover_profile.out | tail -n 1 | awk '{ print $$3 }' | sed -e 's/^\([0-9]*\).*$$/\1/g' && rm ./cover_profile.out) && \
    	echo "Current coverage: $$CUR_COVERAGE%" && \
    	if [[ $$CUR_COVERAGE -lt $(UNIT_COVERAGE_MIN) ]]; then \
    		echo "Coverage is not enough: $$CUR_COVERAGE% < $(UNIT_COVERAGE_MIN)%"; \
    		exit 1; \
    	else \
    		echo "Coverage is enough: $$CUR_COVERAGE% >= $(UNIT_COVERAGE_MIN)%"; \
    	fi

bench-all:
	go test -bench=. -benchmem ./...