FROM golang:1.17.6-alpine as builder
RUN apk add --no-cache ca-certificates make gcc musl-dev linux-headers git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p build
RUN go build -o ./build github.com/ericlee42/metis-bridge-faucet

FROM alpine:latest
COPY --from=builder /app/build/metis-bridge-faucet /usr/local/bin/
ENTRYPOINT [ "metis-bridge-faucet" ]
