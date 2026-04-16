bin-name := "upload-to-spotify"

build:
    mkdir -p dist
    go build -o dist/{{bin-name}} .

build-all:
    mkdir -p dist
    GOOS=darwin GOARCH=arm64 go build -o dist/{{bin-name}}-darwin-arm64 .
    GOOS=darwin GOARCH=amd64 go build -o dist/{{bin-name}}-darwin-amd64 .
    GOOS=windows GOARCH=amd64 go build -o dist/{{bin-name}}-windows-amd64.exe .
    GOOS=windows GOARCH=arm64 go build -o dist/{{bin-name}}-windows-arm64.exe .
    GOOS=linux GOARCH=amd64 go build -o dist/{{bin-name}}-linux-amd64 .
    GOOS=linux GOARCH=arm64 go build -o dist/{{bin-name}}-linux-arm64 .

sign:
    ./scripts/sign-and-notarize.sh dist/{{bin-name}}-darwin-arm64 dist/{{bin-name}}-darwin-amd64

release tag: build-all sign
    gh release create "{{tag}}" dist/{{bin-name}}-* \
        --title "{{tag}}" \
        --generate-notes

test:
    go test ./...
