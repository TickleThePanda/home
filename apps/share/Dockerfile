FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod *.go *.html ./
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /share .

FROM scratch
COPY --from=build /share /share
USER 65532:65532
ENTRYPOINT ["/share"]
