#!/bin/sh
# install.sh — install omni-cli from GitHub Releases.
#
# Usage:
#   ./install.sh [--version=<version>] [--prefix=<dir>]
#
# Options:
#   --version=<version>  Release version to install (default: latest release).
#                        A leading "v" is accepted and stripped (v1.0.0 -> 1.0.0).
#                        Equivalent environment variable: VERSION.
#   --prefix=<dir>       Installation directory. Default: /usr/local/bin; when
#                        that is not writable the script falls back to
#                        $HOME/.local/bin and prints a PATH hint.
#   -h, --help           Show this help.
#
# Behavior:
#   1. Detect platform (darwin/linux x amd64/arm64; Windows users are pointed
#      at the Releases page).
#   2. Download <base>/<tag>/omni-cli_<version>_<os>_<arch>.tar.gz and the
#      release's checksums.txt into a temp dir (cleaned up on exit).
#   3. Verify the artifact's sha256 against checksums.txt. Verification is
#      mandatory: if no checksum tool (sha256sum or shasum) is available, or
#      the checksum does not match, the script exits non-zero and installs
#      nothing.
#   4. Extract omni-cli into the prefix (atomic rename, no partial files) and
#      self-check by running <prefix>/omni-cli --version via absolute path.
#
# Internal/test hooks (NOT part of the public interface):
#   OMNI_CLI_INSTALL_BASE  Override the download base URL
#                          (default: https://github.com/itopener/omni-cli/releases/download).
#   OMNI_CLI_INSTALL_API   Override the latest-release API URL
#                          (default: https://api.github.com/repos/itopener/omni-cli/releases/latest).

set -u

DOWNLOAD_BASE="${OMNI_CLI_INSTALL_BASE:-https://github.com/itopener/omni-cli/releases/download}"
LATEST_API="${OMNI_CLI_INSTALL_API:-https://api.github.com/repos/itopener/omni-cli/releases/latest}"
DEFAULT_PREFIX="/usr/local/bin"
FALLBACK_PREFIX="${HOME:-}/.local/bin"
RELEASES_PAGE="https://github.com/itopener/omni-cli/releases"

WORK_DIR=

die() {
    echo "install.sh: error: $1" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Usage: install.sh [--version=<version>] [--prefix=<dir>]

Install omni-cli from GitHub Releases (https://github.com/itopener/omni-cli).

Options:
  --version=<version>  Version to install (default: latest release).
                       A leading "v" is accepted and stripped (v1.0.0 -> 1.0.0).
                       Equivalent environment variable: VERSION.
  --prefix=<dir>       Installation directory (default: /usr/local/bin; falls
                       back to $HOME/.local/bin when the default is not
                       writable).
  -h, --help           Show this help.

The downloaded tarball is verified against the release's checksums.txt
(sha256, mandatory). On checksum mismatch, or when no checksum tool
(sha256sum / shasum) is available, the installer exits non-zero and
installs nothing.
EOF
}

# Print $1 with any leading "v" stripped.
normalize_version() {
    case $1 in
        v*) printf '%s\n' "${1#v}" ;;
        *) printf '%s\n' "$1" ;;
    esac
}

# Set PLATFORM_OS and PLATFORM_ARCH from uname, or die.
detect_platform() {
    kernel=$(uname -s) || die "uname -s failed"
    machine=$(uname -m) || die "uname -m failed"
    case $kernel in
        Darwin) PLATFORM_OS=darwin ;;
        Linux) PLATFORM_OS=linux ;;
        MINGW* | MSYS* | CYGWIN* | Windows_NT)
            die "Windows is not supported by this installer; download the windows binary from $RELEASES_PAGE"
            ;;
        *)
            die "unsupported operating system: $kernel (see $RELEASES_PAGE)"
            ;;
    esac
    case $machine in
        x86_64 | amd64) PLATFORM_ARCH=amd64 ;;
        arm64 | aarch64) PLATFORM_ARCH=arm64 ;;
        *) die "unsupported architecture: $machine (supported: amd64, arm64)" ;;
    esac
}

# Print the latest version (v stripped), resolved via $LATEST_API.
resolve_latest_version() {
    api_json="$WORK_DIR/latest.json"
    http_code=$(curl -sS -o "$api_json" -w '%{http_code}' "$LATEST_API" 2>/dev/null)
    curl_rc=$?
    if [ "$curl_rc" -ne 0 ]; then
        die "failed to query latest release ($LATEST_API): curl exit $curl_rc.
To skip the API lookup, pin a version: VERSION=<version> ./install.sh"
    fi
    case $http_code in
        200) ;;
        403 | 429)
            die "GitHub API refused the latest-release query (HTTP $http_code); this is usually API rate limiting.
Retry later, or pin a version to skip the API lookup: VERSION=<version> ./install.sh"
            ;;
        *)
            die "failed to query latest release ($LATEST_API): HTTP $http_code.
You can pin a version to skip the API lookup: VERSION=<version> ./install.sh"
            ;;
    esac
    tag_line=$(grep -o '"tag_name": *"[^"]*"' "$api_json" 2>/dev/null | head -n 1)
    if [ -z "$tag_line" ]; then
        die "could not parse tag_name from the latest-release response of $LATEST_API"
    fi
    tag=$(printf '%s\n' "$tag_line" | sed 's/"tag_name": *"//; s/"$//')
    if [ -z "$tag" ]; then
        die "latest-release response of $LATEST_API contains an empty tag_name"
    fi
    normalize_version "$tag"
}

# Install $1 as $2/omni-cli (copy to a temp name, then atomic rename).
# Returns non-zero on failure and leaves no partial file behind.
install_bin() {
    src=$1
    dest_dir=$2
    dest_tmp="$dest_dir/.omni-cli.tmp.$$"
    if ! cp "$src" "$dest_tmp" 2>/dev/null; then
        rm -f "$dest_tmp" 2>/dev/null
        return 1
    fi
    if ! chmod 755 "$dest_tmp" 2>/dev/null; then
        rm -f "$dest_tmp" 2>/dev/null
        return 1
    fi
    if ! mv -f "$dest_tmp" "$dest_dir/omni-cli" 2>/dev/null; then
        rm -f "$dest_tmp" 2>/dev/null
        return 1
    fi
    return 0
}

main() {
    flag_version=
    flag_prefix=
    for arg in "$@"; do
        case $arg in
            --version=*) flag_version=${arg#--version=} ;;
            --prefix=*) flag_prefix=${arg#--prefix=} ;;
            -h | --help) usage; exit 0 ;;
            *)
                usage >&2
                die "unknown argument: $arg (use --help)"
                ;;
        esac
    done

    command -v curl >/dev/null 2>&1 \
        || die "curl is required but was not found in PATH"

    # Resolve the version: --version flag, then VERSION env, then latest API.
    if [ -n "$flag_version" ]; then
        version=$(normalize_version "$flag_version")
        [ -n "$version" ] || die "invalid version: '$flag_version'"
    elif [ -n "${VERSION:-}" ]; then
        version=$(normalize_version "$VERSION")
        [ -n "$version" ] || die "invalid version: '$VERSION'"
    else
        version=""
    fi
    case $version in
        "") ;;  # resolved below via the API
        *[!A-Za-z0-9._-]*)
            die "invalid version: '$version' (allowed: letters, digits, '.', '_', '-')"
            ;;
    esac

    WORK_DIR=$(mktemp -d) || die "mktemp -d failed"
    trap 'rm -rf "$WORK_DIR"' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM

    if [ -z "$version" ]; then
        # Note: resolve_latest_version runs in a subshell, so its die() only
        # exits the substitution; propagate the failure to the main shell.
        version=$(resolve_latest_version) || exit 1
        echo "==> latest release: $version"
    fi

    detect_platform
    tag="v$version"
    archive="omni-cli_${version}_${PLATFORM_OS}_${PLATFORM_ARCH}.tar.gz"
    base_url="$DOWNLOAD_BASE/$tag"

    echo "==> downloading $base_url/$archive"
    curl -fsSL "$base_url/$archive" -o "$WORK_DIR/$archive" \
        || die "download failed: $base_url/$archive"
    curl -fsSL "$base_url/checksums.txt" -o "$WORK_DIR/checksums.txt" \
        || die "download failed: $base_url/checksums.txt"

    # Mandatory sha256 verification. checksums.txt lists every artifact of the
    # release, so pick the line for exactly this archive and compare hashes.
    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$WORK_DIR/$archive" | cut -c1-64) \
            || die "sha256sum failed for $archive"
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$WORK_DIR/$archive" | cut -c1-64) \
            || die "shasum failed for $archive"
    else
        die "no sha256 checksum tool found (need sha256sum or shasum); refusing to install without verification"
    fi
    if [ -z "$actual" ]; then
        die "sha256 computation produced no result for $archive"
    fi
    expected=$(grep "$archive\$" "$WORK_DIR/checksums.txt" 2>/dev/null | head -n 1 | cut -c1-64)
    if [ "${#expected}" -ne 64 ]; then
        die "no checksum entry for $archive found in checksums.txt"
    fi
    if [ "$actual" != "$expected" ]; then
        die "sha256 mismatch for $archive
  expected: $expected
  actual:   $actual
Nothing was installed."
    fi
    echo "==> checksum OK"

    tar -xzf "$WORK_DIR/$archive" -C "$WORK_DIR" \
        || die "failed to extract $archive"
    [ -f "$WORK_DIR/omni-cli" ] \
        || die "archive $archive does not contain an omni-cli binary"

    # Choose destination: explicit --prefix (created if needed), else default
    # with fallback to $HOME/.local/bin.
    dest="$flag_prefix"
    used_fallback=0
    if [ -n "$dest" ]; then
        mkdir -p "$dest" 2>/dev/null || die "mkdir -p $dest failed"
        install_bin "$WORK_DIR/omni-cli" "$dest" \
            || die "could not install into $dest (check permissions)"
    else
        dest="$DEFAULT_PREFIX"
        if ! install_bin "$WORK_DIR/omni-cli" "$dest"; then
            if [ -z "$FALLBACK_PREFIX" ]; then
                die "no write permission for $dest and no HOME fallback available"
            fi
            echo "==> no write permission for $dest, falling back to $FALLBACK_PREFIX" >&2
            mkdir -p "$FALLBACK_PREFIX" 2>/dev/null \
                || die "mkdir -p $FALLBACK_PREFIX failed"
            install_bin "$WORK_DIR/omni-cli" "$FALLBACK_PREFIX" \
                || die "could not install into $FALLBACK_PREFIX"
            dest="$FALLBACK_PREFIX"
            used_fallback=1
        fi
    fi
    dest_abs=$(cd "$dest" 2>/dev/null && pwd) \
        || die "could not resolve absolute path of $dest"
    echo "==> installed: $dest_abs/omni-cli"

    if [ "$used_fallback" -eq 1 ]; then
        echo "note: $dest_abs is the install directory; make sure it is on your PATH:"
        echo "  export PATH=\"$dest_abs:\$PATH\""
    fi

    # Self-check by absolute path.
    version_out=$("$dest_abs/omni-cli" --version 2>&1)
    self_rc=$?
    if [ "$self_rc" -ne 0 ]; then
        die "self-check failed: $dest_abs/omni-cli --version exited with $self_rc"
    fi
    echo "==> self-check OK: $version_out"
}

main "$@"
