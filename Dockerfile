FROM golang:1.13 AS builder
ENV GO111MODULE=on

WORKDIR /go/src/github.com/midcontinentcontrols/kinder
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy remainder of source tree
COPY . .

# Build binary
WORKDIR /go/src/github.com/midcontinentcontrols/kinder/cmd/kinder
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build

###########################################################
## Runtime Container
###########################################################
FROM alpine:3.11.5
COPY --from=builder /go/src/github.com/midcontinentcontrols/kinder/cmd/kinder/kinder /usr/local/bin
CMD ["kinder"]
