FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

WORKDIR /src
ARG TARGETOS=linux
ARG TARGETARCH
COPY go.mod ./
COPY . .
RUN apk add --no-cache ca-certificates
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOFLAGS=-buildvcs=false go build -trimpath -ldflags="-s -w" -o /out/pk-deploy-controlplane ./cmd/pk-deploy-controlplane
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOFLAGS=-buildvcs=false go build -trimpath -ldflags="-s -w" -o /out/pk-deploy-worker ./cmd/pk-deploy-worker

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/pk-deploy-controlplane /usr/local/bin/pk-deploy-controlplane
COPY --from=build /out/pk-deploy-worker /usr/local/bin/pk-deploy-worker

USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/pk-deploy-controlplane"]
