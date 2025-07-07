FROM golang:1.24 AS builder
WORKDIR /
RUN apt-get update && apt-get install unzip
RUN wget https://github.com/protocolbuffers/protobuf/releases/download/v29.1/protoc-29.1-linux-x86_64.zip
RUN unzip protoc-29.1-linux-x86_64.zip -d /
WORKDIR /build
COPY api/ api
COPY pkg/ pkg
COPY third_party/ third_party
COPY go.mod/ .
COPY go.sum/ .

RUN go get google.golang.org/grpc
RUN go install google.golang.org/grpc
RUN go get github.com/golang/protobuf/protoc-gen-go
RUN go install github.com/golang/protobuf/protoc-gen-go

RUN mkdir -p /build/pkg/api/v1
RUN /bin/protoc --proto_path=/build/api/proto/v1 --proto_path=/build/third_party --go_out=plugins=grpc:/build/pkg/api/v1 config-service.proto

RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go test ./...
WORKDIR /build/pkg/cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build

FROM alpine:latest
LABEL maintainer=nmaas@lists.geant.org
COPY --from=builder /build/pkg/cmd/server/server /go/bin/nmaas-janitor
ENTRYPOINT /go/bin/nmaas-janitor -port $SERVER_PORT -token $GITLAB_TOKEN -url $GITLAB_URL
