# Image that provides cross compilation tooling.
FROM --platform=$BUILDPLATFORM rancher/mirrored-tonistiigi-xx:1.6.1 AS xx

# Minimal BCI base image, as recommended by RFD 0070.
FROM registry.suse.com/bci/bci-nano:16.1 AS base

# The AppCo Go image has no package manager, so the tools needed by xx
# (file, flock, sed, grep) are installed from BCI base.
# Its SLE version must match the one AppCo Go is based on.
FROM --platform=$BUILDPLATFORM registry.suse.com/bci/bci-base:15.7 AS tools
RUN zypper --non-interactive refresh && \
    zypper --non-interactive --installroot /chroot install --no-recommends \
        file sed grep util-linux

# bci-nano has no shell to create the gke-operator user or the directories it
# needs, so the files are prepared here and copied into the final image.
FROM --platform=$BUILDPLATFORM registry.suse.com/bci/bci-base:15.7 AS user-setup
COPY --from=base /etc/passwd /final/etc/passwd
COPY --from=base /etc/group /final/etc/group
RUN echo "gke-operator:x:1007:1007::/home/gke-operator:/usr/bin/false" >> /final/etc/passwd && \
    echo "gke-operator:x:1007:" >> /final/etc/group && \
    install -dv -m 0755 -o 1007 -g 1007 /final/etc/rancher/ssl

# Builds with the Go compiler from the AppCo Go image, as recommended by
# RFD 0070. Pinned to MAJOR.MINOR.PATCH so the toolchain is deterministic and
# bumped by Renovate.
FROM --platform=$BUILDPLATFORM registry.suse.com/rancher/appco-go:1.27.3 AS builder

COPY --from=tools /chroot /

WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY ./controller ./controller
COPY ./pkg ./pkg
COPY ./main.go ./main.go

# Copy xx scripts to your build stage
COPY --from=xx / /

ARG TARGETPLATFORM
ARG COMMIT
ARG VERSION
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod \
    xx-go build -ldflags \
           "-X github.com/rancher/gke-operator/pkg/version.GitCommit=${COMMIT} \
            -X github.com/rancher/gke-operator/pkg/version.Version=${VERSION}" \
       -o /out/gke-operator && \
       xx-verify /out/gke-operator

FROM base AS final

COPY --from=user-setup /final/ /
COPY --from=builder /out/gke-operator /usr/bin/gke-operator

ENV KUBECONFIG="/home/gke-operator/.kube/config"
# gke-operator is a static Go binary, which loads every certificate file in
# SSL_CERT_DIR, so no c_rehash step is needed for additional CAs.
ENV SSL_CERT_DIR="/etc/rancher/ssl"

USER 1007
ENTRYPOINT ["/usr/bin/gke-operator"]
