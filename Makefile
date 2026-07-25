SHELL := /bin/zsh
PROJECT_NAME?=bunyan

# makefile listing from: http://stackoverflow.com/questions/4219255/how-do-you-get-the-list-of-targets-in-a-makefile
default-goal:
	@echo "viable targets:"
	@$(MAKE) -pRrq -f $(lastword $(MAKEFILE_LIST)) : 2>/dev/null | awk -v RS= -F: '/^# File/,/^# Finished Make data base/ {if ($$1 !~ "^[#.]") {print $$1}}' | sort | egrep -v -e '^[^[:alnum:]]' -e '^$@$$' | xargs

# update dependencies and vendor them
update_deps:
	go get -u && go mod vendor

install_static_tools:
	@# don't check if commands already exist as we want to stay as up to date as possible
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/google/osv-scanner/cmd/osv-scanner@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest

# easier than writing a loop construct. just target specific requirements
# https://github.com/securego/gosec
# go install github.com/securego/gosec/v2/cmd/gosec@latest
exists_gosec: ; @which gosec > /dev/null

# https://github.com/google/osv-scanner
# go install github.com/google/osv-scanner/cmd/osv-scanner@latest
exists_osv: ; @which osv-scanner > /dev/null

# https://staticcheck.io/docs/getting-started/#distribution-packages
# go install honnef.co/go/tools/cmd/staticcheck@latest
exists_go_static: ; @which staticcheck > /dev/null

# security check
do_gosec: exists_gosec
	gosec ./...

# security check
do_osv: exists_osv
	@#osv-scanner -r ./
	osv-scanner --lockfile=./go.mod

# You can use staticcheck -explain <check> to get a helpful description of a check.
# security check, performance
do_go_static: exists_go_static
	staticcheck ./...

# https://go.dev/doc/articles/race_detector
do_race:
	go build -race main.go

checks: do_go_static do_gosec do_osv do_race
	@# correctness check
	go vet ./...

test_coverage:
	go test --coverprofile cover.out ./...
	go tool cover -func=cover.out

test_profile:
	go test --coverprofile $(target)_cover.out ./$(target)
	go tool cover -func=$(target)_cover.out

tests:
	go test ./...