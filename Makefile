.ONESHELL:
.PHONY: paruz

paruz:
	go build -o paruz -ldflags "-s -w" ./cmd/paruz

clean:
	rm paruz
