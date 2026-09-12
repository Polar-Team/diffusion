# syntax=docker/dockerfile:1
# -------------------------------------------------------------------
# Diffusion MCP Server — multi-arch container image
# ghcr.io/polar-team/diffusion-mcp-server:{version}-{amd64|arm64}
#
# Includes:
#   - diffusion CLI binary (compiled from source)
#   - diffusion-mcp Python server (FastMCP)
#   - Docker CE CLI (for container management tools)
#
# Mount your project at /project:
#   docker run --rm -i \
#     -v /var/run/docker.sock:/var/run/docker.sock \
#     -v $(pwd):/project \
#     ghcr.io/polar-team/diffusion-mcp-server:latest
# -------------------------------------------------------------------

# DO NOT EDIT BELOW - VERSIONS MANAGED BY update-mcp-dockerfile.yml
ARG GO_VERSION="1.27.1"
ARG PYTHON_VERSION="3.13.15-slim"
ARG UV_VERSION="0.12.10"
# DO NOT EDIT ABOVE - VERSIONS MANAGED BY update-mcp-dockerfile.yml

# ---- stage 1: compile the diffusion Go binary ----
FROM golang:${GO_VERSION} AS go-builder

ARG DIFFUSION_VERSION="dev"

WORKDIR /build
# Copy Go module files first for layer caching
COPY diffusion_src/go.mod diffusion_src/go.sum ./
RUN go mod download

# Copy the rest of the source and build
COPY diffusion_src/ ./
RUN CGO_ENABLED=0 go build \
  -ldflags "-s -w -X diffusion/internal/cli.Version=${DIFFUSION_VERSION}" \
  -o /usr/local/bin/diffusion \
  ./cmd/diffusion

# ---- stage 2: install Python deps into a venv ----
FROM python:${PYTHON_VERSION} AS py-builder

ARG UV_VERSION

COPY pyproject.toml README.md /src/
COPY diffusion_mcp/ /src/diffusion_mcp/

RUN pip install --no-cache-dir "uv==${UV_VERSION}" && \
  uv venv /opt/venv && \
  uv pip install --python /opt/venv/bin/python /src && \
  rm -rf /root/.cache /tmp/*

FROM python:${PYTHON_VERSION} AS docker-builder

# Install Docker CLI (needed for container management tools)
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# hadolint  ignore=DL3008
RUN apt-get update && \
  apt-get install -y --no-install-recommends \
  ca-certificates \
  curl \
  gnupg && \
  install -m 0755 -d /etc/apt/keyrings && \
  curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
  chmod a+r /etc/apt/keyrings/docker.gpg && \
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" > /etc/apt/sources.list.d/docker.list && \
  apt-get update && \
  apt-get install -y --no-install-recommends docker-ce-cli && \
  apt-get purge -y gnupg && \
  apt-get autoremove -y && \
  rm -rf /var/lib/apt/lists/* /tmp/*

# ---- stage 3: assemble runtime filesystem ----
# Everything is assembled here and then flattened into a single layer in
# stage 4. Flattening removes the cross-layer duplicates that the upstream
# python:*-slim base image ships with (libssl/libcrypto, dpkg/debconf state),
# which otherwise cap `dive` efficiency at ~94%.
FROM python:${PYTHON_VERSION} AS runtime

# Copy Docker CLI binary from docker-builder (statically linked, no extra libs needed)
COPY --from=docker-builder /usr/bin/docker /usr/bin/docker

# Copy diffusion binary from Go builder
COPY --from=go-builder /usr/local/bin/diffusion /usr/local/bin/diffusion

# Copy Python venv from py-builder
COPY --from=py-builder /opt/venv /opt/venv

# Trim runtime-irrelevant files inherited from the base image:
#   - apt/dpkg/debconf caches, logs and backup databases
#   - base-image pip/ensurepip (the venv in /opt/venv carries its own tooling)
#   - documentation
RUN set -eux; \
  rm -rf /var/lib/apt/lists/* \
  /var/cache/apt/* \
  /var/cache/debconf/*-old \
  /var/cache/ldconfig/aux-cache \
  /var/lib/dpkg/*-old \
  /var/log/* \
  /tmp/* /var/tmp/* \
  /root/.cache \
  /usr/share/doc /usr/share/man /usr/share/info; \
  rm -rf /usr/local/lib/python3*/site-packages/pip* \
  /usr/local/lib/python3*/ensurepip \
  /usr/local/bin/pip*; \
  find / -xdev -type d -name __pycache__ -prune -exec rm -rf '{}' + ; \
  ldconfig

# ---- stage 4: final single-layer image ----
FROM scratch

LABEL maintainer="Daniel Dalavurak"
LABEL org.label-schema.vendor="Polar Team"
LABEL org.label-schema.name="diffusion-mcp-server"
LABEL org.label-schema.description="MCP server for managing and troubleshooting Diffusion CLI environments"

COPY --from=runtime / /

# Re-declare environment lost when starting FROM scratch
ENV LANG="C.UTF-8" \
  PYTHONDONTWRITEBYTECODE="1" \
  PATH="/opt/venv/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin"

# The MCP server needs access to the host Docker socket (/var/run/docker.sock)
# and the project directory for reading diffusion.toml, molecule.yml, etc.
#
# Usage:
#   docker run --rm -i \
#     -v /var/run/docker.sock:/var/run/docker.sock \
#     -v /path/to/project:/project \
#     ghcr.io/polar-team/diffusion-mcp-server:latest
#
# The project mount at /project is the default WORKDIR, so diffusion CLI
# commands auto-detect diffusion.toml and the MCP server's _find_project_root
# resolves config files from there.
WORKDIR /project

# Default: run the MCP server on stdio transport
ENTRYPOINT ["diffusion-mcp"]
