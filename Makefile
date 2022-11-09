TARGET=go-imap-backup.exe

all: $(TARGET)

$(TARGET): *.go
	go build

commit: $(TARGET)
	go fmt
	golangci-lint run
	go mod tidy
	go build
	go test

test:
	go test

clean:
	rm -f $(TARGET) $(TARGET).exe
