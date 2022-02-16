FROM --platform=$BUILDPLATFORM golang:1.17 as builder

ARG TAG
ARG COMMIT
ARG REPO_INFO
ARG DATE
ARG TARGETOS TARGETARCH
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
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GO111MODULE=on go build -ldflags "-X 'github.com/keikoproj/addon-manager/pkg/version.GitCommit=${COMMIT}' -X 'github.com/keikoproj/addon-manager/pkg/version.BuildDate=${DATE}'" -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot AS distroless
WORKDIR /
COPY --from=builder /workspace/manager .
USER nonroot:nonroot

ENTRYPOINT ["/manager"]
