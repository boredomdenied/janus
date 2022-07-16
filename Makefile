GO ?= $(shell command -v go 2> /dev/null)
GO_BUILD_FLAGS ?=

build:
	$(GO) build $(GO_BUILD_FLAGS) -trimpath -o out/janus-srv gitlab.operationuplift.work/operations/development/janus/cmd/janus-srv

run: build
	test -s config/$(USER)-dev.env \
		&& echo "config/$(USER)-dev.env exists, sourcing." && source config/$(USER)-dev.env && out/janus-srv \
		|| echo "config/$(USER)-dev.env does not exist, skipping." && out/janus-srv

clean:
	rm out/janus-srv
