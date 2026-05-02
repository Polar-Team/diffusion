# syntax=docker/dockerfile:1
# -------------------------------------------------------------------
# Diffusion MCP Server — multi-arch container image
# ghcr.io/polar-team/diffusion-mcp-server:{version}-{amd64|arm64}
# -------------------------------------------------------------------

ARG PYTHON_VERSION="3.13-slim"

# ---- builder: install deps into a venv so the final stage stays small ----
FROM python:${PYTHON_VERSION} AS builder

COPY pyproject.toml README.md /src/
COPY diffusion_mcp/ /src/diffusion_mcp/

RUN pip install --no-cache-dir uv && \
    uv venv /opt/venv && \
    uv pip install --python /opt/venv/bin/python /src && \
    rm -rf /root/.cache /tmp/*

# ---- final: minimal runtime image ----
FROM python:${PYTHON_VERSION}

LABEL maintainer="Daniel Dalavurak"
LABEL org.label-schema.vendor="Polar Team"
LABEL org.label-schema.name="diffusion-mcp-server"
LABEL org.label-schema.description="MCP server for managing and troubleshooting Diffusion CLI environments"

# Install Docker CLI (needed for container management tools)
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
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

# Clean up debconf templates to reduce image size
RUN rm -rf /var/cache/debconf/* && \
    find /var/lib/dpkg -name "*.dat-old" -delete

# Copy venv from builder
COPY --from=builder /opt/venv /opt/venv

ENV PATH="/opt/venv/bin:$PATH"

# The MCP server needs access to the host Docker socket (/var/run/docker.sock),
# which is typically owned by root:docker. We run as root by default to avoid
# GID mismatch issues across different host configurations (Docker Desktop,
# standard Linux installs, rootless Docker all use different GIDs).
#
# To run as non-root, mount the socket and pass the host docker GID:
#   docker run --rm -i \
#     -v /var/run/docker.sock:/var/run/docker.sock \
#     --group-add $(stat -c '%g' /var/run/docker.sock) \
#     --user 1000:1000 \
#     ghcr.io/polar-team/diffusion-mcp-server:latest
WORKDIR /opt/mcp

# Default: run the MCP server on stdio transport
ENTRYPOINT ["diffusion-mcp"]
