

linux_build:
	GOOS="linux" GOARCH="amd64" go build -ldflags "-s" -o ./bin/diffusion.exe .

win_build:
	GOOS="windows" GOARCH="amd64" go build -ldflags "-s" -o ./bin/diffusion.exe .
