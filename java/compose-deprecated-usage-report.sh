#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE' >&2
Usage:
  ./compose-deprecated-usage-report.sh \
    --usage-report <path> \
    --classpath-file <path> \
    [--repo-url <url>] \
    [--output <path>]
USAGE
}

if [[ $# -eq 0 ]]; then
  usage
  exit 1
fi

if [[ "$1" == "--help" || "$1" == "-h" ]]; then
  usage
  exit 0
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
pom_file="$script_dir/pom.xml"
mdep_plugin="org.apache.maven.plugins:maven-dependency-plugin:3.8.1"
classpath_file="$(mktemp)"
cleanup() {
  rm -f "$classpath_file"
}
trap cleanup EXIT

mvn -q -f "$pom_file" -DskipTests compile
mvn -q -f "$pom_file" "$mdep_plugin:build-classpath" -DincludeScope=runtime -Dmdep.outputFile="$classpath_file"

runtime_cp="$(cat "$classpath_file"):$script_dir/target/classes"
java -cp "$runtime_cp" DependencyDeprecatedUsageReportCli "$@"
