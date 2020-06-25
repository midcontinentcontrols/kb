FROM golang:1.13
ENV GO111MODULE=on

WORKDIR /go/src/github.com/midcontinentcontrols/kindest
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .

WORKDIR /go/src/github.com/midcontinentcontrols/kindest/test/e2e
CMD ["bash", "-c", "go test -v -timeout 99999s"]
