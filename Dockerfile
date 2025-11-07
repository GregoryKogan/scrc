# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25.3

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o scrc ./cmd/scrc

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/scrc ./scrc

ENTRYPOINT ["./scrc"]

