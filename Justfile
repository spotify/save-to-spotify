bin-name := "save-to-spotify"
bin-version := `git describe --tags --always 2>/dev/null | sed 's/^v//' || echo "dev"`
bin-commit := `git rev-parse --short HEAD`
ldflags := "-X github.com/spotify/save-to-spotify/cmd.commit=" + bin-commit + " -X github.com/spotify/save-to-spotify/cmd.version=" + bin-version

build:
    mkdir -p dist
    go build -ldflags "{{ldflags}}" -o dist/{{bin-name}} .

build-all:
    mkdir -p dist
    GOOS=darwin GOARCH=arm64 go build -ldflags "{{ldflags}}" -o dist/{{bin-name}}-darwin-arm64 .
    GOOS=darwin GOARCH=amd64 go build -ldflags "{{ldflags}}" -o dist/{{bin-name}}-darwin-amd64 .
    GOOS=windows GOARCH=amd64 go build -ldflags "{{ldflags}}" -o dist/{{bin-name}}-windows-amd64.exe .
    GOOS=windows GOARCH=arm64 go build -ldflags "{{ldflags}}" -o dist/{{bin-name}}-windows-arm64.exe .
    GOOS=linux GOARCH=amd64 go build -ldflags "{{ldflags}}" -o dist/{{bin-name}}-linux-amd64 .
    GOOS=linux GOARCH=arm64 go build -ldflags "{{ldflags}}" -o dist/{{bin-name}}-linux-arm64 .

test:
    go test ./...

sign: build-all
    ./scripts/sign-and-notarize.sh dist/{{bin-name}}-darwin-arm64 dist/{{bin-name}}-darwin-amd64

release version:
    git tag v{{ trim_start_matches(version, "v") }}
    git push origin v{{ trim_start_matches(version, "v") }}

clawhub-publish:
    ./scripts/clawhub-publish
