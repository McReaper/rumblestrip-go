# syntax=docker/dockerfile:1

# Rumblestrip's image: Go at the toolchain go.mod pins, every tool tools.txt
# pins, and rumblestrip at the commit it's built from. CI and the agent's
# machine run the checks in it, so both run the same tools.
#
# The versions come from go.mod and tools.txt. Only Ubuntu's and PowerShell's
# are here. Each download is checked against a checksum its publisher gives:
# go.dev's for Go, the digest GitHub recorded at upload for a release asset.

# renovate: datasource=docker depName=ubuntu
ARG UBUNTU=ubuntu:26.04@sha256:065aa8626841f16057b78b027b464cd59c4fac36c204c72dcbf48d66f5fa8a96

FROM ${UBUNTU} AS fetch
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git jq xz-utils \
    && rm -rf /var/lib/apt/lists/*
COPY go.mod tools.txt /pins/
# WHY: GitHub's API refuses an address after 60 calls an hour without a token;
# CI passes its own, read-only, as a build secret no layer keeps.
COPY --chmod=0755 <<'EOF' /usr/local/bin/release-asset
#!/bin/sh
# release-asset <owner/repo> <tag> <asset>: downloads the asset and checks it
# against the digest GitHub recorded when it was uploaded.
set -eu
auth=
if [ -s /run/secrets/github_token ]; then
  auth="Authorization: Bearer $(cat /run/secrets/github_token)"
fi
digest=$(curl -fsSL ${auth:+-H "$auth"} "https://api.github.com/repos/$1/releases/tags/$2" \
  | jq -r --arg name "$3" '.assets[] | select(.name == $name) | .digest')
case $digest in
  sha256:*) ;;
  *) echo "release-asset: GitHub records no sha256 for $1 $2 $3" >&2; exit 1 ;;
esac
curl -fsSL -o "$3" "https://github.com/$1/releases/download/$2/$3"
echo "${digest#sha256:}  $3" | sha256sum -c -
EOF

# Go, at the toolchain go.mod pins.
FROM fetch AS go
WORKDIR /tmp/go
RUN set -eu; \
    v=$(awk '$1 == "toolchain" {print $2}' /pins/go.mod); \
    test -n "$v"; \
    file="$v.linux-amd64.tar.gz"; \
    sum=$(curl -fsSL 'https://go.dev/dl/?mode=json&include=all' | jq -r --arg f "$file" '.[].files[] | select(.filename == $f) | .sha256'); \
    curl -fsSL -o "$file" "https://go.dev/dl/$file"; \
    echo "$sum  $file" | sha256sum -c -; \
    tar -C /usr/local -xzf "$file"
ENV PATH=/usr/local/go/bin:$PATH

# The Go tools tools.txt pins, installed as CI installs them.
FROM go AS gotools
WORKDIR /tmp/gotools
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    grep -v '^#' /pins/tools.txt | grep @ | CGO_ENABLED=0 GOBIN=/out xargs -n1 go install

# zizmor and shellcheck, at the versions tools.txt pins.
FROM fetch AS bintools
WORKDIR /tmp/bintools
RUN --mount=type=secret,id=github_token,required=false \
    set -eu; \
    version() { awk -v name="$1" '$1 == name {print $2}' /pins/tools.txt; }; \
    mkdir /out; \
    v=$(version zizmor); \
    release-asset zizmorcore/zizmor "v$v" zizmor-x86_64-unknown-linux-gnu.tar.gz; \
    mkdir zizmor; \
    tar -xzf zizmor-x86_64-unknown-linux-gnu.tar.gz -C zizmor; \
    find zizmor -type f -name zizmor -exec install -m 0755 {} /out/ \; ; \
    test -x /out/zizmor; \
    v=$(version shellcheck); \
    release-asset koalaman/shellcheck "v$v" "shellcheck-v$v.linux.x86_64.tar.xz"; \
    tar -xJf "shellcheck-v$v.linux.x86_64.tar.xz" --strip-components=1 -C /out "shellcheck-v$v/shellcheck"

# PowerShell, and the PSScriptAnalyzer tools.txt pins.
FROM fetch AS pwsh
# renovate: datasource=github-releases depName=PowerShell/PowerShell extractVersion=^v(?<version>.+)$
ARG PWSH_VERSION=7.7.0
ENV DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1 POWERSHELL_TELEMETRY_OPTOUT=1
WORKDIR /tmp/pwsh
RUN --mount=type=secret,id=github_token,required=false \
    set -eu; \
    release-asset PowerShell/PowerShell "v$PWSH_VERSION" "powershell-$PWSH_VERSION-linux-x64.tar.gz"; \
    mkdir -p /opt/microsoft/powershell/7; \
    tar -xzf "powershell-$PWSH_VERSION-linux-x64.tar.gz" -C /opt/microsoft/powershell/7; \
    chmod 0755 /opt/microsoft/powershell/7/pwsh; \
    v=$(awk '$1 == "PSScriptAnalyzer" {print $2}' /pins/tools.txt); \
    /opt/microsoft/powershell/7/pwsh -NoLogo -NoProfile -NonInteractive -Command \
      "\$ErrorActionPreference = 'Stop'; Install-PSResource -Name PSScriptAnalyzer -Version $v -Repository PSGallery -Scope AllUsers -TrustRepository -Quiet"

# rumblestrip, at the commit of the build's context: with .git there, Go
# stamps that commit's version, which the review sheet's footer shows.
FROM go AS build
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/rumblestrip ./cmd/rumblestrip

FROM ${UBUNTU}
# CI passes the date, so this layer runs again each day with Ubuntu's
# security fixes rather than staying in the build cache. pebble comes with
# the base image; nothing here runs it, and it only adds CVEs to the scan.
# gcc and libc6-dev: the tests run with -race, which needs cgo.
ARG APT_REFRESH=manual
RUN echo "apt refresh: ${APT_REFRESH}" \
    && apt-get update \
    && apt-get upgrade -y \
    && rm -f /usr/bin/pebble \
    && apt-get install -y --no-install-recommends ca-certificates curl gcc git jq libc6-dev \
    && rm -rf /var/lib/apt/lists/*
COPY --from=go /usr/local/go /usr/local/go
COPY --from=pwsh /opt/microsoft/powershell /opt/microsoft/powershell
COPY --from=pwsh /usr/local/share/powershell /usr/local/share/powershell
COPY --from=gotools /out/ /usr/local/bin/
COPY --from=bintools /out/ /usr/local/bin/
COPY --from=build /out/rumblestrip /usr/local/bin/rumblestrip
# WHY: the image runs as whichever user owns the repository it's given (CI's
# runner, the developer on Linux), so Go's caches and HOME take any user, and
# git trusts a repository whatever its owner.
ENV PATH=/usr/local/go/bin:/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    GOPATH=/go \
    GOCACHE=/go/cache/go-build \
    GOLANGCI_LINT_CACHE=/go/cache/golangci-lint \
    HOME=/home/rumblestrip \
    DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1 \
    POWERSHELL_TELEMETRY_OPTOUT=1
RUN ln -s /opt/microsoft/powershell/7/pwsh /usr/local/bin/pwsh \
    && useradd --create-home --home-dir /home/rumblestrip --shell /bin/bash rumblestrip \
    && mkdir -p /go /work \
    && chmod 1777 /go /home/rumblestrip /work \
    && git config --system --add safe.directory '*'
# Each tool starts: a missing library fails the build, not a check.
RUN export HOME=/tmp/smoke GOCACHE=/tmp/smoke/go-build GOLANGCI_LINT_CACHE=/tmp/smoke/golangci-lint \
    && mkdir "$HOME" \
    && rumblestrip --help >/dev/null \
    && go version \
    && golangci-lint version \
    && zizmor --version \
    && shellcheck --version \
    && pwsh -NoLogo -NoProfile -NonInteractive -Command "Import-Module PSScriptAnalyzer; (Get-Module PSScriptAnalyzer).Version.ToString()" \
    && rm -rf /tmp/smoke
LABEL org.opencontainers.image.source=https://github.com/McReaper/rumblestrip-go \
      org.opencontainers.image.description="Rumblestrip's checks with every tool they run" \
      org.opencontainers.image.licenses=MIT
USER rumblestrip
WORKDIR /work
CMD ["rumblestrip", "--help"]
