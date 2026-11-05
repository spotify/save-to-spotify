#!/usr/bin/env bash
#
# Smoke-test a released binary: assert `--version` reports the expected
# version and that `doctor` runs cleanly. Shared by the Release workflow's
# pre-publish verify job and the post-publish verify-release workflow.
#
# Usage: verify-binary.sh <binary> <expected-version>

set -euo pipefail

bin="$1"
version="$2"

out="$("$bin" --version)"
echo "$out"
case "$out" in
  *" ${version} ("*) ;;
  *)
    echo "::error::--version output does not report ${version}"
    exit 1
    ;;
esac

"$bin" doctor
