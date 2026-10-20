#!/usr/bin/env bash
# antlr.sh — run the ANTLR tool with the given arguments.
#
#   ./scripts/antlr.sh -Dlanguage=Go ... grammar/*.g4
#
# Uses the self-contained "complete" jar (tool + runtime + deps), downloaded
# once to .cache/ and verified against a pinned SHA-256. Needs Java 11+ on
# PATH; if it is missing, prints the install command for this OS.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

version="4.13.2"
# Same hash from antlr.org and Maven Central; SHA-1 matches Maven's published one.
sha256="eae2dfa119a64327444672aff63e9ec35a20180dc5b8090b7a6ab85125df4d76"
url="https://repo1.maven.org/maven2/org/antlr/antlr4/${version}/antlr4-${version}-complete.jar"
jar="$root/.cache/antlr-${version}-complete.jar"
min_java=11

# java_hint prints how to install Eclipse Temurin (formerly AdoptOpenJDK) 21.
java_hint() {
  local repo="https://adoptium.net/installation/linux/"
  case "$(uname -s)" in
    Darwin) echo "  brew install --cask temurin@21" ;;
    MINGW* | MSYS* | CYGWIN*) echo "  winget install EclipseAdoptium.Temurin.21.JRE" ;;
    Linux)
      local id="" like="" pm=""
      if [[ -r /etc/os-release ]]; then
        id="$(. /etc/os-release && echo "${ID:-}")"
        like="$(. /etc/os-release && echo "${ID_LIKE:-}")"
      fi
      case " $id $like " in
        *" debian "* | *" ubuntu "*) pm="sudo apt install" ;;
        *" fedora "* | *" rhel "* | *" centos "*) pm="sudo dnf install" ;;
        *" opensuse"* | *" suse "*) pm="sudo zypper install" ;;
        *" alpine "*) pm="sudo apk add" ;;
      esac
      if [[ -n "$pm" ]]; then
        echo "  add the Adoptium repository ($repo), then:"
        echo "  $pm temurin-21-jre"
      else
        echo "  Eclipse Temurin 21: $repo"
      fi
      ;;
    *) echo "  Eclipse Temurin 21: https://adoptium.net/temurin/releases/" ;;
  esac
}

if ! command -v java >/dev/null 2>&1; then
  echo "antlr: Java not found on PATH. Install Eclipse Temurin 21:" >&2
  java_hint >&2
  exit 1
fi
# "1.8.0_x" → 8, "21.0.2" → 21
java_major="$(java -version 2>&1 | sed -nE 's/.*version "(1\.)?([0-9]+).*/\2/p' | head -1)"
if [[ -z "$java_major" || "$java_major" -lt "$min_java" ]]; then
  echo "antlr: Java ${min_java}+ required, found ${java_major:-unknown}. Install Eclipse Temurin 21:" >&2
  java_hint >&2
  exit 1
fi

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

if [[ ! -f "$jar" ]]; then
  echo "antlr: downloading ANTLR ${version}..." >&2
  mkdir -p "$(dirname "$jar")"
  tmp="$(mktemp "$jar.XXXXXX")"
  trap 'rm -f "$tmp"' EXIT
  curl -fsSL -o "$tmp" "$url"
  mv "$tmp" "$jar"
fi
# Checked on every run, so a corrupt or altered cached jar is caught too.
got="$(sha256_of "$jar")"
if [[ "$got" != "$sha256" ]]; then
  rm -f "$jar"
  echo "antlr: checksum mismatch for $url (removed; rerun to download again)" >&2
  echo "  want $sha256" >&2
  echo "  got  $got" >&2
  exit 1
fi

exec java -jar "$jar" "$@"
