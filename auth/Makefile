.PHONY: build run test clean lint migrate up down

build:
	go build -o bin/auth-service cmd/auth-service/main.go

run:
	go run cmd/auth-service/main.go

test:
	go test -v ./...

clean:
	rm -rf bin/

lint:
	go vet ./...

up:
	docker-compose -f deployments/compose/docker-compose.yml up -d

down:
	docker-compose -f deployments/compose/docker-compose.yml down

migrate:
	go run cmd/migrate/main.go up
