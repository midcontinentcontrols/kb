FROM golang:1.14 AS builder
ENV GO111MODULE=on
WORKDIR /go/src/github.com/midcontinentcontrols/kindest
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY kaniko kaniko
WORKDIR /go/src/github.com/midcontinentcontrols/kindest/kaniko/cmd/kindestkaniko
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build

FROM gcr.io/kaniko-project/executor:debug AS kaniko
FROM alpine:3.11.6
ENV KUBECTL=v1.17.0
RUN wget -O /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/${KUBECTL}/bin/linux/amd64/kubectl \
    && chmod +x /usr/local/bin/kubectl
RUN mkdir /kaniko
COPY --from=kaniko /kaniko/executor /kaniko/executor
COPY --from=builder /go/src/github.com/midcontinentcontrols/kindest/kaniko/cmd/kindestkaniko/kindestkaniko /usr/local/bin/kindestkaniko
CMD ["kindestkaniko"]
