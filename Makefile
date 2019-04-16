loracli:
	go build -o loracli cli/*.go
all:
	go build -o gui

requirements:
	dep ensure -v

dev-requirements:
	go get -u github.com/golang/dep/cmd/dep
