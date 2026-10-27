#!/bin/sh
set -eu
PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
CHECK_DIR="$PROJECT_ROOT/work/conformance"
mkdir -p "$CHECK_DIR"
cd "$PROJECT_ROOT/sparqlite"
DEPENDENCY_DIR=$(go list -m -f '{{.Dir}}' github.com/tggo/goRDFlib)
if [ ! -d "$CHECK_DIR/engine" ]; then
  mkdir -p "$CHECK_DIR/engine"
  cp -R "$DEPENDENCY_DIR/." "$CHECK_DIR/engine/"
  chmod -R u+w "$CHECK_DIR/engine"
fi
if [ ! -d "$CHECK_DIR/rdf-tests/.git" ]; then
  git clone --no-checkout https://github.com/w3c/rdf-tests.git "$CHECK_DIR/rdf-tests"
fi
git -C "$CHECK_DIR/rdf-tests" checkout --detach bfefa770267ec0e5b6e45764bbca4ee43789ba72
if [ ! -e "$CHECK_DIR/engine/testdata/w3c/rdf-tests" ]; then
  ln -s "$CHECK_DIR/rdf-tests" "$CHECK_DIR/engine/testdata/w3c/rdf-tests"
fi
cd "$CHECK_DIR/engine"
GOWORK=off CGO_ENABLED=0 go test -json ./sparql -run 'TestW3C($|Dataset|Update)' -count=1
