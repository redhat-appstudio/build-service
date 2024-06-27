# Build the manager binary
# For more details and updates, refer to
# https://catalog.redhat.com/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690
FROM registry.access.redhat.com/ubi9/go-toolset:1.21.10-1 AS builder

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY controllers/ controllers/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

# Use ubi-minimal as minimal base image to package the manager binary
# For more details and updates, refer to
# https://catalog.redhat.com/software/containers/ubi9/ubi-minimal/615bd9b4075b022acc111bf5
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.4
COPY --from=builder /opt/app-root/src/manager /
USER 65532:65532

# Required for ecosystem-cert-preflight-checks
# https://access.redhat.com/documentation/en-us/red_hat_software_certification/2024/html-single/red_hat_openshift_software_certification_policy_guide/index#assembly-requirements-for-container-images_openshift-sw-cert-policy-introduction
COPY LICENSE /licenses/LICENSE

LABEL description="Konflux Build Service operator"
LABEL io.k8s.description="Konflux Build Service operator"
LABEL io.k8s.display-name="konflux-build-service-operator"
LABEL io.openshift.tags="konflux"
LABEL summary="Konflux Build Service"
LABEL name="konflux-build-service"
LABEL com.redhat.component="konflux-build-service-operator"

ENTRYPOINT ["/manager"]
