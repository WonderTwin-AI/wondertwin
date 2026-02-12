FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY ../pkg/ ./pkg/
COPY twin-stripe/ ./twin-stripe/
WORKDIR /build/twin-stripe
RUN go build -o /twin-stripe ./cmd/twin-stripe/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /twin-stripe /usr/local/bin/twin-stripe
EXPOSE 12111
ENTRYPOINT ["twin-stripe"]
