ARG SOURCE_BINARY=bin/manager

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:latest
WORKDIR /
COPY ${SOURCE_BINARY} .
EXPOSE 8080 8443
ENTRYPOINT ["/manager"]
CMD ["--debug=true"]
