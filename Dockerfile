FROM golang:1.24.0-alpine3.21 AS build_deps

RUN apk add --no-cache git

WORKDIR /workspace

COPY go.mod .
COPY go.sum .

RUN go mod download

FROM build_deps AS build

COPY . .

RUN GCO_ENABLED=0 go build -o webhook -ldflags '-w -extldflags "-static"' .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=build /workspace/webhook /usr/local/bin/webhook

ENTRYPOINT ["webhook"]