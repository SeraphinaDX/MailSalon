.PHONY: build test

build:
	go build -o MailSalon ./cmd/MailSalon

test:
	go test ./...
