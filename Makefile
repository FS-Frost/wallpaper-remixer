build:
	GOOS=${WALLPAPER_REMIXER_BUILD_OS} \
	GOARCH=${WALLPAPER_REMIXER_BUILD_ARCH} \
		go build -ldflags "-X 'main.sourceDir=$(CURDIR)'" -o "${WALLPAPER_REMIXER_BUILD_BIN}" .

run:
	"${WALLPAPER_REMIXER_BUILD_BIN}"

test:
	go test ./...
