FROM golang:alpine3.10 as builder

RUN mkdir /app
COPY *.go /app/
WORKDIR /app
RUN go mod init main
RUN go get
RUN go build -o guide2go

FROM alpine:3.10

COPY --from=builder /app/guide2go /usr/local/bin/guide2go
COPY sample-config.yaml /config/sample-config.yaml

CMD [ "guide2go", "--config", "/config/sample-config.yaml" ]