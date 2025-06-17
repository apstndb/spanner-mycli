build:
	go build

clean:
	rm -f spanner-mycli
	rm -rf dist/
	go clean -testcache

run:
	./spanner-mycli -p ${PROJECT} -i ${INSTANCE} -d ${DATABASE}

test:
	go test ./...

test-verbose:
	go test -v ./...

fasttest:
	go test -short ./...

fasttest-verbose:
	go test -short -v ./...

lint:
	golangci-lint run
