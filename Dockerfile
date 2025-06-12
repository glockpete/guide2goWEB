FROM golang:1.23-alpine AS builder

RUN apk add --no-cache ca-certificates git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o guide2go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /app/guide2go .
COPY sample-config.yaml /config/sample-config.yaml
CMD ["./guide2go", "--config", "/config/sample-config.yaml"]