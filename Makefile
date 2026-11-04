.PHONY: fmt test verify build images helm-check docs-check e2e

fmt:
	gofmt -w cmd internal

docs-check:
	go run ./cmd/quality docs .

test:
	mkdir -p reports
	go test -race -covermode=atomic -coverpkg=./... -coverprofile=reports/coverage.out ./...
	go run ./cmd/quality coverage reports/coverage.out > reports/coverage.txt
	go tool cover -html=reports/coverage.out -o reports/coverage.html
	cat reports/coverage.txt

helm-check:
	helm lint charts/greatfirewall -f examples/values-demo.yaml
	helm template gfw charts/greatfirewall -n greatfirewall-system -f examples/values-demo.yaml > /dev/null

verify: docs-check
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	$(MAKE) test helm-check

build:
	mkdir -p bin
	go build -trimpath -o bin/operator ./cmd/operator
	go build -trimpath -o bin/mock-api ./cmd/mock-api

images:
	docker build --target operator -t greatfirewall/operator:0.1.0 .
	docker build --target mock-api -t greatfirewall/mock-api:0.1.0 .

e2e:
	bash scripts/e2e.sh
