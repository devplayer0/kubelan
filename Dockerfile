FROM golang:1.16-alpine3.14 AS builder
RUN apk --no-cache add gcc musl-dev

WORKDIR /usr/local/kubelan
COPY go.* ./
RUN go mod download

COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
RUN mkdir bin/ && go build -o bin/ ./cmd/...


FROM alpine:3.14

RUN apk --no-cache add iproute2

COPY --from=builder /usr/local/kubelan/bin/* /usr/local/bin/

ENV KL_LOG_LEVEL= \
    KL_IP= \
    KL_SERVICES= \
    KL_VXLAN_INTERFACE= \
    KL_VXLAN_VNI= \
    KL_VXLAN_PORT=
ENTRYPOINT ["/usr/local/bin/kubelan"]
