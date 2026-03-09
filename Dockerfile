FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=

RUN go build \
    -ldflags "-s -w -X helmdex/internal/appinfo.Version=${VERSION} -X helmdex/internal/appinfo.Commit=${COMMIT}" \
    -o /helmdex ./cmd/helmdex

FROM alpine:3.21

RUN apk add --no-cache ca-certificates git helm

COPY --from=builder /helmdex /usr/local/bin/helmdex

ENTRYPOINT ["helmdex"]
