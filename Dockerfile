FROM golang:1.15-alpine AS builder
RUN apk --no-cache add gcc musl-dev

WORKDIR /usr/local/kubelan
COPY go.* ./
RUN go mod download

COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
RUN mkdir bin/ && go build -o bin/ ./cmd/...


FROM alpine:3.12

COPY --from=builder /usr/local/kubelan/bin/* /usr/local/bin/

ENV KL_LOG_LEVEL=
ENV KL_IP=
ENV KL_SERVICES=
ENV KL_INTERFACE=
ENV KL_VID=
ENTRYPOINT ["/usr/local/bin/kubelan"]
