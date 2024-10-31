build:
	go build

cross-compile:
	gox -os="linux" -arch="386 amd64 arm arm64" -output="dist/{{.Dir}}_{{.OS}}_{{.Arch}}/{{.Dir}}"
	gox -os="darwin" -arch="386 amd64" -output="dist/{{.Dir}}_{{.OS}}_{{.Arch}}/{{.Dir}}"
	gox -os="windows" -arch="386 amd64" -output="dist/{{.Dir}}_{{.OS}}_{{.Arch}}/{{.Dir}}"

package:
	goxc

release: clean cross-compile package
	ghr -draft -token ${GITHUB_TOKEN} ${VERSION} dist/snapshot/
	@echo "released as draft"

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
