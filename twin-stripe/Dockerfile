FROM golang:1.25-alpine@sha256:5caaf1cca9dc351e13deafbc3879fd4754801acba8653fa9540cea125d01a71f AS builder
WORKDIR /build
COPY ../pkg/ ./pkg/
COPY twin-stripe/ ./twin-stripe/
WORKDIR /build/twin-stripe
RUN go build -o /twin-stripe ./cmd/twin-stripe/

FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d
RUN apk add --no-cache ca-certificates
COPY --from=builder /twin-stripe /usr/local/bin/twin-stripe
EXPOSE 12111
RUN adduser -D -u 1000 twin
USER twin
ENTRYPOINT ["twin-stripe"]
