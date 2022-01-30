FROM golang:1.17 as builder

ARG TAG
ARG COMMIT
ARG REPO_INFO
WORKDIR /workspace

ADD go.mod .
ADD go.sum .
RUN go mod download

COPY pkg/ pkg/
COPY api/ api/
COPY cmd/ cmd/
COPY controllers/ controllers/
COPY main.go main.go
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o bin/manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot AS distroless
WORKDIR /bin
COPY --from=builder /workspace/bin/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
