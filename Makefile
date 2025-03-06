build:
	go build

clean:
	rm -f spanner-mycli
	rm -rf dist/
	go clean -testcache

run:
	./spanner-mycli -p ${PROJECT} -i ${INSTANCE} -d ${DATABASE}

test:
	go test -v ./...

fasttest:
	go test --tags skip_slow_test -v ./...

lint:
	golangci-lint run
