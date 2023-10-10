VERSION ?= $(shell git describe --tags --always --dirty --match=v* 2> /dev/null || cat $(CURDIR)/.version 2> /dev/null || echo v0)
BLDVER = module:$(MODULE),version:$(VERSION),build:$(CIRCLE_BUILD_NUM)
BASE = $(CURDIR)
MODULE = juno

.PHONY: all $(MODULE) install
all: version $(MODULE)

$(MODULE):| $(BASE)
	@protoc --go_out . --go_opt paths=source_relative --go-grpc_out . --go-grpc_opt paths=source_relative ./proto/v1/juno.proto
	@GO111MODULE=on go build -v -trimpath -ldflags "-X google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn" -o $(BASE)/bin/$@

$(BASE):
	@mkdir -p $(dir $@)

install:
	@GO111MODULE=on go install -ldflags "-X google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn" -v

.PHONY: custom docker deploy

# The rule that is called by our root Makefile during CI builds.
custom: test docker deploy

test:
	go test -v $(BASE)/cloudrun/juno/... -cover -race -mod=vendor

docker:
	cp $(BASE)/cloudrun/juno/dockerfile.juno $(BASE)/
	docker build -f dockerfile.juno --rm -t $(MODULE):$(CIRCLE_SHA1) --build-arg version="$(BLDVER)" .

deploy:
	@chmod +x $(BASE)/cloudrun/juno/deploy.sh
	NAME=$(MODULE) $(BASE)/cloudrun/juno/deploy.sh

.PHONY: deploy-by-helper docker-by-helper pushdeploy-by-helper

# The rule that is called by our root Makefile via helper.
deploy-by-helper: docker-by-helper pushdeploy-by-helper

docker-by-helper:
	cp $(BASE)/cloudrun/juno/dockerfile.juno $(BASE)/
	docker build -f dockerfile.juno --rm -t $(MODULE):$(OUCHAN_HELPER_IMAGETAG) --build-arg version="$(BLDVER)" .

pushdeploy-by-helper:
	@chmod +x $(BASE)/cloudrun/juno/deploy.sh
	LEX=true NAME=$(MODULE) $(BASE)/cloudrun/juno/deploy.sh

.PHONY: version list
version:
	@echo "Version: $(VERSION)"

list:
	@$(MAKE) -pRrq -f $(lastword $(MAKEFILE_LIST)) : 2>/dev/null | awk -v RS= -F: '/^# File/,/^# Finished Make data base/ {if ($$1 !~ "^[#.]") {print $$1}}' | sort | egrep -v -e '^[^[:alnum:]]' -e '^$@$$' | xargs
