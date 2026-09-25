BINARY  := predictor
GO      := go
LDFLAGS := -ldflags="-s -w"

.PHONY: build test bench vet clean docker run backtest optimize dashboard

## build: compile to a static binary
build:
	$(GO) build $(LDFLAGS) -o $(BINARY) .

## test: run all tests with race detector
test:
	$(GO) test ./... -v -count=1 -race

## bench: run benchmarks
bench:
	$(GO) test ./... -bench=. -benchmem -run=^$

## vet: run static analysis
vet:
	$(GO) vet ./...

## clean: remove build artefacts
clean:
	rm -f $(BINARY)

## docker: build a production Docker image
docker:
	docker build -t $(BINARY):latest .

## run: live analysis with default coins
run: build
	./$(BINARY)

## backtest: 1-year walk-forward backtest on BTC, ETH, SOL
backtest: build
	./$(BINARY) --backtest --days 365

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'

## optimize: walk-forward optimisation over the last year
optimize:
	$(GO) run . --optimize --days 365

## dashboard: regenerate web/data.json and serve the dashboard on http://localhost:8000
dashboard:
	$(GO) run . --days 365 --export web/data.json
	cd web && python3 -m http.server 8000
