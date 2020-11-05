check:
	go fmt ./...
	go vet ./...
	golint ./...

install:
	go install ...
	go mod tidy

cert:
	mkcert -install
	mkcert -cert-file example.crt -key-file example.key example.com localhost 127.0.0.1
