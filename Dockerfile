# Locus MCP Server — on-demand analysis container
#
#   make deploy
#   podman run --rm -i -v /path/to/repo:/path/to/repo:rbind \
#     locus:$(VERSION) serve --transport http --addr :8081

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git xz-utils \
    gcc g++ make cmake \
    python3 python3-venv \
    clangd-18 universal-ctags \
    && ln -sf /usr/bin/clangd-18 /usr/bin/clangd \
    && rm -rf /var/lib/apt/lists/*

ARG GO_VERSION=1.25.8
RUN ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz
ENV GOBIN=/usr/local/bin
ENV PATH="/usr/local/go/bin:${PATH}"
RUN go install golang.org/x/tools/gopls@latest

ARG NODE_VERSION=20.18.1
RUN ARCH=$(dpkg --print-architecture) && \
    case "$ARCH" in amd64) NODE_ARCH="x64" ;; arm64) NODE_ARCH="arm64" ;; *) exit 1 ;; esac && \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" | \
    tar -C /usr/local --strip-components=1 -xJ
RUN npm install -g typescript typescript-language-server pyright

ENV RUSTUP_HOME=/usr/local/rustup CARGO_HOME=/usr/local/cargo
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y \
    --default-toolchain stable --profile minimal --component rust-analyzer
ENV PATH="/usr/local/cargo/bin:${PATH}"

# Core LSP servers (installed above):
#   gopls           — Go
#   rust-analyzer   — Rust
#   pyright         — Python
#   typescript-language-server — TypeScript/JavaScript
#   clangd          — C/C++
#
# Optional LSP servers (install manually for additional language support):
#   jdtls           — Java (Eclipse JDT Language Server)
#   kotlin-language-server — Kotlin
#   omnisharp       — C#
#   sourcekit-lsp   — Swift (requires Xcode)
#   zls             — Zig

# Run as non-root — LSP servers don't need root, and orphaned child
# processes from killed containers shouldn't run as root on the host.
RUN useradd -m -s /bin/bash locus
USER locus

# Runtime cargo/rust paths must be writable by the running user.
# The install-time CARGO_HOME (/usr/local/cargo) is root-owned.
# Override at runtime to user-writable location.
ENV CARGO_HOME=/tmp/cargo RUSTUP_HOME=/tmp/rustup

ENV GOMEMLIMIT=1GiB
ENV GOGC=50

COPY locus /usr/local/bin/locus

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD ["locus", "version"]

ENTRYPOINT ["locus"]
CMD ["serve"]
