FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY ../twinkit/ ./twinkit/
COPY twin-loyaltylion/ ./twin-loyaltylion/
WORKDIR /build/twin-loyaltylion
RUN go build -o /twin-loyaltylion ./cmd/twin-loyaltylion/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /twin-loyaltylion /usr/local/bin/twin-loyaltylion
EXPOSE 8090
ENTRYPOINT ["twin-loyaltylion"]
