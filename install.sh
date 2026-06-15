#!/bin/sh
# Sloppy Joe one-line installer.
#
#   curl -fsSL https://raw.githubusercontent.com/jakecastillo/sloppy-joe/main/install.sh | sh
#
# Detects OS/arch, downloads the matching GitHub release archive plus
# checksums.txt, verifies the SHA-256 checksum (and the keyless cosign
# signature over checksums.txt when cosign is installed), then installs the
# `sloppy` and `sloppyd` binaries into a prefix (default /usr/local/bin).
#
# Configuration (flags override environment):
#   --version vX.Y.Z   VERSION    release tag to install (default: latest)
#   --prefix DIR       PREFIX     install directory  (default: /usr/local/bin)
#   --repo owner/name  REPO       GitHub repo        (default: jakecastillo/sloppy-joe)
#
# This script is self-contained POSIX sh; it needs only standard userland
# (uname, tar, mktemp, sha256sum or shasum) plus curl or wget.
set -eu

REPO="${REPO:-jakecastillo/sloppy-joe}"
PREFIX="${PREFIX:-/usr/local/bin}"
VERSION="${VERSION:-}"
PROJECT="sloppy-joe"
BINARIES="sloppy sloppyd"

# --- output helpers ----------------------------------------------------------

info() {
	printf '==> %s\n' "$*"
}

warn() {
	printf 'warning: %s\n' "$*" >&2
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<EOF
Sloppy Joe installer

Usage: install.sh [--version TAG] [--prefix DIR] [--repo OWNER/NAME]

Options:
  --version TAG     Release tag to install, e.g. v1.2.3 (default: latest)
  --prefix DIR      Directory to install binaries into (default: /usr/local/bin)
  --repo OWNER/NAME GitHub repository to fetch from (default: jakecastillo/sloppy-joe)
  -h, --help        Show this help and exit

Environment variables VERSION, PREFIX and REPO are honored as defaults.
EOF
}

# --- argument parsing --------------------------------------------------------

while [ "$#" -gt 0 ]; do
	case "$1" in
	--version)
		[ "$#" -ge 2 ] || die "--version requires an argument"
		VERSION="$2"
		shift 2
		;;
	--version=*)
		VERSION="${1#*=}"
		shift
		;;
	--prefix)
		[ "$#" -ge 2 ] || die "--prefix requires an argument"
		PREFIX="$2"
		shift 2
		;;
	--prefix=*)
		PREFIX="${1#*=}"
		shift
		;;
	--repo)
		[ "$#" -ge 2 ] || die "--repo requires an argument"
		REPO="$2"
		shift 2
		;;
	--repo=*)
		REPO="${1#*=}"
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage >&2
		die "unknown argument: $1"
		;;
	esac
done

# --- prerequisites -----------------------------------------------------------

have() {
	command -v "$1" >/dev/null 2>&1
}

if have curl; then
	DOWNLOADER=curl
elif have wget; then
	DOWNLOADER=wget
else
	die "need curl or wget to download release artifacts"
fi

have tar || die "need tar to extract the release archive"

# download URL DEST
download() {
	if [ "$DOWNLOADER" = curl ]; then
		curl -fsSL -o "$2" "$1"
	else
		wget -qO "$2" "$1"
	fi
}

# fetch URL -> stdout
fetch() {
	if [ "$DOWNLOADER" = curl ]; then
		curl -fsSL "$1"
	else
		wget -qO- "$1"
	fi
}

# --- OS / arch detection -----------------------------------------------------

detect_os() {
	os="$(uname -s)"
	case "$os" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	*)
		die "unsupported OS '$os'; on Windows download the .zip from https://github.com/${REPO}/releases"
		;;
	esac
}

detect_arch() {
	arch="$(uname -m)"
	case "$arch" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*)
		die "unsupported architecture '$arch'; supported: amd64, arm64"
		;;
	esac
}

OS="$(detect_os)"
ARCH="$(detect_arch)"

# --- version resolution ------------------------------------------------------

# Latest release tag from the GitHub API (no jq dependency).
latest_version() {
	body="$(fetch "https://api.github.com/repos/${REPO}/releases/latest")" ||
		die "could not query latest release for ${REPO}"
	tag="$(printf '%s\n' "$body" |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
		head -n 1)"
	[ -n "$tag" ] || die "could not determine the latest release tag for ${REPO}"
	echo "$tag"
}

if [ -z "$VERSION" ]; then
	info "Resolving latest release of ${REPO}"
	VERSION="$(latest_version)"
fi

# goreleaser archives use the version without a leading 'v'.
VER_NUM="${VERSION#v}"

ARCHIVE="${PROJECT}_${VER_NUM}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

info "Installing ${PROJECT} ${VERSION} (${OS}/${ARCH}) to ${PREFIX}"

# --- download into a temp dir ------------------------------------------------

TMPDIR_INSTALL="$(mktemp -d 2>/dev/null || mktemp -d -t sloppy-joe)" ||
	die "could not create a temporary directory"

cleanup() {
	rm -rf "$TMPDIR_INSTALL"
}
trap cleanup EXIT INT TERM

info "Downloading ${ARCHIVE}"
download "${BASE_URL}/${ARCHIVE}" "${TMPDIR_INSTALL}/${ARCHIVE}" ||
	die "failed to download ${ARCHIVE} (does release ${VERSION} have a ${OS}/${ARCH} build?)"

info "Downloading checksums.txt"
download "${BASE_URL}/checksums.txt" "${TMPDIR_INSTALL}/checksums.txt" ||
	die "failed to download checksums.txt for ${VERSION}"

# --- optional cosign signature verification ----------------------------------

verify_signature() {
	have cosign || {
		warn "cosign not found; skipping signature verification of checksums.txt"
		return 0
	}
	if ! download "${BASE_URL}/checksums.txt.sig" "${TMPDIR_INSTALL}/checksums.txt.sig" 2>/dev/null ||
		! download "${BASE_URL}/checksums.txt.pem" "${TMPDIR_INSTALL}/checksums.txt.pem" 2>/dev/null; then
		warn "cosign signature/certificate not published for ${VERSION}; skipping signature verification"
		return 0
	fi
	info "Verifying cosign signature of checksums.txt"
	cosign verify-blob \
		--certificate "${TMPDIR_INSTALL}/checksums.txt.pem" \
		--signature "${TMPDIR_INSTALL}/checksums.txt.sig" \
		--certificate-identity-regexp '.*' \
		--certificate-oidc-issuer-regexp '.*' \
		"${TMPDIR_INSTALL}/checksums.txt" >/dev/null 2>&1 ||
		die "cosign signature verification of checksums.txt failed"
	info "cosign signature OK"
}

verify_signature

# --- checksum verification ---------------------------------------------------

verify_checksum() {
	expected="$(grep " ${ARCHIVE}\$" "${TMPDIR_INSTALL}/checksums.txt" |
		awk '{print $1}' | head -n 1)"
	[ -n "$expected" ] ||
		die "no checksum entry for ${ARCHIVE} in checksums.txt"

	if have sha256sum; then
		actual="$(sha256sum "${TMPDIR_INSTALL}/${ARCHIVE}" | awk '{print $1}')"
	elif have shasum; then
		actual="$(shasum -a 256 "${TMPDIR_INSTALL}/${ARCHIVE}" | awk '{print $1}')"
	else
		die "need sha256sum or shasum to verify the download"
	fi

	[ "$expected" = "$actual" ] ||
		die "checksum mismatch for ${ARCHIVE}: expected ${expected}, got ${actual}"
	info "checksum OK"
}

verify_checksum

# --- extract & install -------------------------------------------------------

info "Extracting ${ARCHIVE}"
tar -xzf "${TMPDIR_INSTALL}/${ARCHIVE}" -C "$TMPDIR_INSTALL" ||
	die "failed to extract ${ARCHIVE}"

# Install with sudo only when the prefix is not writable by the current user.
SUDO=""
if [ ! -d "$PREFIX" ]; then
	if ! mkdir -p "$PREFIX" 2>/dev/null; then
		if have sudo; then
			SUDO="sudo"
			$SUDO mkdir -p "$PREFIX" || die "could not create ${PREFIX}"
		else
			die "cannot create ${PREFIX}; rerun with --prefix to a writable dir or install sudo"
		fi
	fi
elif [ ! -w "$PREFIX" ]; then
	if have sudo; then
		SUDO="sudo"
	else
		die "no write permission for ${PREFIX}; rerun with --prefix to a writable dir or install sudo"
	fi
fi

for bin in $BINARIES; do
	src="${TMPDIR_INSTALL}/${bin}"
	[ -f "$src" ] || die "expected binary '${bin}' missing from ${ARCHIVE}"
	chmod +x "$src"
	$SUDO install -m 0755 "$src" "${PREFIX}/${bin}" 2>/dev/null ||
		$SUDO cp "$src" "${PREFIX}/${bin}" ||
		die "failed to install ${bin} to ${PREFIX}"
	info "installed ${PREFIX}/${bin}"
done

info "Done. Ensure ${PREFIX} is on your PATH, then run: sloppy --help"
