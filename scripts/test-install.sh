#!/usr/bin/env bash
# scripts/test-install.sh — test driver for install.sh.
#
# The driver itself is bash; the script under test (install.sh) is STRICT POSIX
# and is always executed with /bin/dash (never bash), except case 5 which only
# runs syntax checks.
#
# Every case builds its own fake release server: a python3 http.server rooted
# at a temp directory that contains
#   api-latest.json                       fake GitHub "latest release" payload
#   releases/download/v1.0.0/omni-cli_1.0.0_<os>_<arch>.tar.gz
#   releases/download/v1.0.0/checksums.txt  (real sha256 + a decoy line)
# The tarball holds an executable fake omni-cli that prints
# "omni-cli version 1.0.0".
#
# Cases:
#   1. dash success path: VERSION=1.0.0 + --prefix=<tmp>/bin -> exit 0,
#      binary exists and `omni-cli --version` prints 1.0.0.
#   2. Version sources must all compose the correct URL (a wrong URL is a 404
#      server-side and fails the install):
#        a) env VERSION=v1.0.0          (leading v must be stripped)
#        b) flag --version=1.0.0        (flag form)
#        c) neither given               (latest resolved via OMNI_CLI_INSTALL_API)
#   3. Tampered checksums.txt (one hex char flipped) -> non-zero exit, no
#      binary left in the prefix, mismatch reported.
#   4. PATH stub without sha256sum/shasum -> non-zero exit, no binary left.
#   5. Syntax checks: sh -n install.sh and /bin/dash -n install.sh.
#
# Run: bash scripts/test-install.sh   (from anywhere; exits non-zero on any
# failing case).

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INSTALL_SH="$ROOT/install.sh"
DASH=/bin/dash
PYTHON="$(command -v python3 2>/dev/null)"

PASS=0
FAIL=0
FAILED=()

CLEANUP_PIDS=()
CLEANUP_DIRS=()

cleanup() {
    local pid dir
    for pid in ${CLEANUP_PIDS[@]+"${CLEANUP_PIDS[@]}"}; do
        kill "$pid" >/dev/null 2>&1
    done
    for dir in ${CLEANUP_DIRS[@]+"${CLEANUP_DIRS[@]}"}; do
        rm -rf "$dir"
    done
}
trap cleanup EXIT

note() { printf '%s\n' "$*"; }
ok() {
    PASS=$((PASS + 1))
    printf 'PASS: %s\n' "$1"
}
fail_case() {
    FAIL=$((FAIL + 1))
    FAILED+=("$1")
    printf 'FAIL: %s%s\n' "$1" "${2:+ — $2}"
}
show_log() {
    local f
    for f in "$@"; do
        [ -f "$f" ] || continue
        printf -- '---- %s ----\n' "$f"
        tail -n 40 "$f"
    done
}

# ---------------------------------------------------------------- platform

REL_OS=
REL_ARCH=
detect_test_platform() {
    case $(uname -s) in
        Darwin) REL_OS=darwin ;;
        Linux)  REL_OS=linux ;;
        *) printf 'test host OS not supported: %s\n' "$(uname -s)" >&2; exit 2 ;;
    esac
    case $(uname -m) in
        x86_64 | amd64)   REL_ARCH=amd64 ;;
        arm64 | aarch64)  REL_ARCH=arm64 ;;
        *) printf 'test host arch not supported: %s\n' "$(uname -m)" >&2; exit 2 ;;
    esac
}

# ---------------------------------------------------------------- fixtures

# Append "<sha256>  <file>" for $1 (file in cwd) to ./checksums.txt.
hash_one() {
    local h
    if command -v shasum >/dev/null 2>&1; then
        h=$(shasum -a 256 "$1" | cut -d' ' -f1)
    else
        h=$(sha256sum "$1" | cut -d' ' -f1)
    fi
    [ -n "$h" ] || return 1
    printf '%s  %s\n' "$h" "$1" >> checksums.txt
}

# Build a fake v1.0.0 release under $1 (server document root) for the current
# platform. checksums.txt also carries a decoy line for another platform whose
# file is intentionally absent: the installer must verify only its own
# artifact's line.
make_release() {
    local root=$1 tag=v1.0.0 ver=1.0.0 bindir rel decoy_os decoy_arch
    bindir="$root/payload"
    rel="$root/releases/download/$tag"
    mkdir -p "$bindir" "$rel" || return 1
    printf '#!/bin/sh\necho "omni-cli version 1.0.0"\n' > "$bindir/omni-cli"
    chmod +x "$bindir/omni-cli"
    tar -czf "$rel/omni-cli_${ver}_${REL_OS}_${REL_ARCH}.tar.gz" \
        -C "$bindir" omni-cli || return 1
    decoy_os=linux
    decoy_arch=amd64
    if [ "$REL_OS/$REL_ARCH" = "linux/amd64" ]; then
        decoy_os=darwin
        decoy_arch=arm64
    fi
    (
        cd "$rel" || exit 1
        hash_one "omni-cli_${ver}_${REL_OS}_${REL_ARCH}.tar.gz" || exit 1
        printf '%s  omni-cli_%s_%s_%s.tar.gz\n' \
            0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
            "$ver" "$decoy_os" "$decoy_arch" >> checksums.txt
    ) || return 1
    printf '{"tag_name": "v1.0.0", "name": "v1.0.0", "prerelease": false}\n' \
        > "$root/api-latest.json"
}

# ---------------------------------------------------------------- server

SRV_PORT=
SRV_PID=

free_port() {
    "$PYTHON" -c 'import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()'
}

# Start fake release server rooted at $1; sets SRV_PORT / SRV_PID.
start_server() {
    local root=$1 i
    SRV_PORT=$(free_port) || { printf 'cannot allocate free port\n' >&2; exit 2; }
    "$PYTHON" -m http.server "$SRV_PORT" --bind 127.0.0.1 -d "$root" \
        >/dev/null 2>&1 &
    SRV_PID=$!
    CLEANUP_PIDS+=("$SRV_PID")
    i=0
    while [ "$i" -lt 100 ]; do
        if curl -fsS -o /dev/null "http://127.0.0.1:$SRV_PORT/" 2>/dev/null; then
            return 0
        fi
        i=$((i + 1))
        sleep 0.1
    done
    printf 'fake release server failed to start\n' >&2
    exit 2
}

kill_server() {
    if [ -n "$SRV_PID" ]; then
        kill "$SRV_PID" >/dev/null 2>&1
        wait "$SRV_PID" 2>/dev/null
        SRV_PID=
    fi
}

base_url() { printf 'http://127.0.0.1:%s/releases/download' "$SRV_PORT"; }
api_url()  { printf 'http://127.0.0.1:%s/api-latest.json' "$SRV_PORT"; }

# ---------------------------------------------------------------- cases

new_tmp() {
    local d
    d=$(mktemp -d "${TMPDIR:-/tmp}/omni-t5.XXXXXX") || exit 2
    CLEANUP_DIRS+=("$d")
    printf '%s\n' "$d"
}

# Case 1: dash success path with VERSION env + --prefix.
case1() {
    local name='case 1: dash success path (VERSION=1.0.0, --prefix)'
    local tmp srv prefix rc
    tmp=$(new_tmp)
    srv=$(new_tmp)
    if ! make_release "$srv"; then
        fail_case "$name" 'fixture build failed'
        return
    fi
    start_server "$srv"
    prefix="$tmp/bin"
    NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost \
    OMNI_CLI_INSTALL_BASE="$(base_url)" OMNI_CLI_INSTALL_API="$(api_url)" \
    VERSION=1.0.0 "$DASH" "$INSTALL_SH" --prefix="$prefix" \
        >"$tmp/out.log" 2>"$tmp/err.log"
    rc=$?
    if [ "$rc" -eq 0 ] && [ -x "$prefix/omni-cli" ] \
       && [ "$("$prefix/omni-cli" --version)" = 'omni-cli version 1.0.0' ]; then
        ok "$name"
    else
        fail_case "$name" "rc=$rc"
        show_log "$tmp/out.log" "$tmp/err.log"
    fi
    kill_server
}

# Case 2: every version source must compose the right URL (else the fake
# server answers 404 and the install fails).
case2() {
    local name='case 2: version sources compose correct URL (v-prefix env / --version flag / API latest)'
    local tmp srv prefix rc all_ok=1
    tmp=$(new_tmp)
    srv=$(new_tmp)
    if ! make_release "$srv"; then
        fail_case "$name" 'fixture build failed'
        return
    fi
    start_server "$srv"

    # 2a: env VERSION=v1.0.0 — leading v must be stripped.
    prefix="$tmp/bin-a"
    NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost \
    OMNI_CLI_INSTALL_BASE="$(base_url)" OMNI_CLI_INSTALL_API="$(api_url)" \
    VERSION=v1.0.0 "$DASH" "$INSTALL_SH" --prefix="$prefix" \
        >"$tmp/a.out" 2>"$tmp/a.err"
    rc=$?
    if [ "$rc" -ne 0 ] || [ ! -x "$prefix/omni-cli" ]; then
        all_ok=0
        printf '  sub-case 2a (VERSION=v1.0.0) failed: rc=%s\n' "$rc"
        show_log "$tmp/a.out" "$tmp/a.err"
    fi

    # 2b: flag --version=1.0.0, no env.
    prefix="$tmp/bin-b"
    NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost \
    OMNI_CLI_INSTALL_BASE="$(base_url)" OMNI_CLI_INSTALL_API="$(api_url)" \
    "$DASH" "$INSTALL_SH" --version=1.0.0 --prefix="$prefix" \
        >"$tmp/b.out" 2>"$tmp/b.err"
    rc=$?
    if [ "$rc" -ne 0 ] || [ ! -x "$prefix/omni-cli" ]; then
        all_ok=0
        printf '  sub-case 2b (--version=1.0.0) failed: rc=%s\n' "$rc"
        show_log "$tmp/b.out" "$tmp/b.err"
    fi

    # 2c: no version given — latest must be resolved via OMNI_CLI_INSTALL_API
    # (tag v1.0.0, v stripped) .
    prefix="$tmp/bin-c"
    NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost \
    OMNI_CLI_INSTALL_BASE="$(base_url)" OMNI_CLI_INSTALL_API="$(api_url)" \
    "$DASH" "$INSTALL_SH" --prefix="$prefix" \
        >"$tmp/c.out" 2>"$tmp/c.err"
    rc=$?
    if [ "$rc" -ne 0 ] || [ ! -x "$prefix/omni-cli" ]; then
        all_ok=0
        printf '  sub-case 2c (latest via API) failed: rc=%s\n' "$rc"
        show_log "$tmp/c.out" "$tmp/c.err"
    fi

    if [ "$all_ok" -eq 1 ]; then
        ok "$name"
    else
        fail_case "$name" 'one or more sub-cases failed (see above)'
    fi
    kill_server
}

# Case 3: tampered checksum -> abort, nothing installed.
case3() {
    local name='case 3: tampered checksums.txt -> non-zero exit, no binary installed'
    local tmp srv prefix rc cs line hash first newhash name_part
    tmp=$(new_tmp)
    srv=$(new_tmp)
    if ! make_release "$srv"; then
        fail_case "$name" 'fixture build failed'
        return
    fi
    cs="$srv/releases/download/v1.0.0/checksums.txt"
    line=$(sed -n 1p "$cs")
    hash=$(printf '%s\n' "$line" | cut -d' ' -f1)
    name_part=${line#*  }   # filename after the two-space separator
    first=${hash%"${hash#?}"}
    if [ "$first" = "a" ]; then newhash="b${hash#?}"; else newhash="a${hash#?}"; fi
    { printf '%s  %s\n' "$newhash" "$name_part"; sed -n 2p "$cs"; } > "$cs.new" \
        && mv "$cs.new" "$cs"
    start_server "$srv"
    prefix="$tmp/bin"
    NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost \
    OMNI_CLI_INSTALL_BASE="$(base_url)" OMNI_CLI_INSTALL_API="$(api_url)" \
    VERSION=1.0.0 "$DASH" "$INSTALL_SH" --prefix="$prefix" \
        >"$tmp/out.log" 2>"$tmp/err.log"
    rc=$?
    if [ "$rc" -ne 0 ] && [ ! -e "$prefix/omni-cli" ] \
       && grep -qi 'mismatch' "$tmp/err.log"; then
        ok "$name"
    else
        fail_case "$name" "rc=$rc binary_exists=$([ -e "$prefix/omni-cli" ] && echo yes || echo no)"
        show_log "$tmp/out.log" "$tmp/err.log"
    fi
    kill_server
}

# Case 4: PATH without sha256sum/shasum -> abort, nothing installed.
case4() {
    local name='case 4: checksum tool missing from PATH -> non-zero exit, no binary installed'
    local tmp srv stub prefix rc t real
    tmp=$(new_tmp)
    srv=$(new_tmp)
    if ! make_release "$srv"; then
        fail_case "$name" 'fixture build failed'
        return
    fi
    start_server "$srv"
    stub="$tmp/stubbin"
    mkdir -p "$stub"
    for t in sh curl tar uname mktemp rm cp mv chmod mkdir sed grep head cut; do
        real=$(command -v "$t" 2>/dev/null)
        if [ -z "$real" ]; then
            fail_case "$name" "host lacks essential command: $t"
            kill_server
            return
        fi
        ln -s "$real" "$stub/$t"
    done
    # Sanity: the stub must NOT contain any checksum tool.
    if PATH="$stub" command -v sha256sum >/dev/null 2>&1 \
       || PATH="$stub" command -v shasum >/dev/null 2>&1; then
        fail_case "$name" 'stub PATH unexpectedly contains a checksum tool'
        kill_server
        return
    fi
    prefix="$tmp/bin"
    env PATH="$stub" \
        NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost \
        OMNI_CLI_INSTALL_BASE="$(base_url)" OMNI_CLI_INSTALL_API="$(api_url)" \
        VERSION=1.0.0 "$DASH" "$INSTALL_SH" --prefix="$prefix" \
        >"$tmp/out.log" 2>"$tmp/err.log"
    rc=$?
    if [ "$rc" -ne 0 ] && [ ! -e "$prefix/omni-cli" ] \
       && grep -qi 'sha256' "$tmp/err.log"; then
        ok "$name"
    else
        fail_case "$name" "rc=$rc binary_exists=$([ -e "$prefix/omni-cli" ] && echo yes || echo no)"
        show_log "$tmp/out.log" "$tmp/err.log"
    fi
    kill_server
}

# Case 5: POSIX syntax checks.
case5() {
    local name='case 5: syntax checks (sh -n, /bin/dash -n)'
    if sh -n "$INSTALL_SH" && "$DASH" -n "$INSTALL_SH"; then
        ok "$name"
    else
        fail_case "$name" 'syntax check returned non-zero'
    fi
}

# ---------------------------------------------------------------- main

if [ -z "$PYTHON" ]; then
    printf 'python3 is required to run these tests\n' >&2
    exit 2
fi
if ! command -v curl >/dev/null 2>&1; then
    printf 'curl is required to run these tests\n' >&2
    exit 2
fi

detect_test_platform
note "install.sh under test: $INSTALL_SH"
note "host platform: $REL_OS/$REL_ARCH"

case1
case2
case3
case4
case5

printf '\nresult: %d/%d cases passed' "$PASS" "$((PASS + FAIL))"
if [ "$FAIL" -gt 0 ]; then
    printf ' (failed: %s)\n' "${FAILED[*]}"
    exit 1
fi
printf '\n'
exit 0
