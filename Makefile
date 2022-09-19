compile-linux:
	echo "Compiling for Linux OS"
	go env -w GOOS=linux
	go env -w CGO_ENABLED=1
	go build -o linux/rubixgoplatform

compile-windows:
	echo "Compiling for Windows OS"
	go env -w GOOS=windows
	go env -w CGO_ENABLED=1
	go build -o windows/rubixgoplatform.exe

all: compile-linux compile-windows