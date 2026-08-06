#!/usr/bin/env bash

set -e

GO_MOD_PACKAGE="github.com/nodelabs-sdk/nodelabs"

echo "Generating gogo proto code"
cd proto
proto_dirs=$(find . -path -prune -o -name '*.proto' -print0 | xargs -0 -n1 dirname | sort | uniq)
for dir in $proto_dirs; do
  for file in $(find "${dir}" -maxdepth 1 -name '*.proto'); do
    # Only generate gogo proto for files with go_package pointing to our module (not api/)
    if grep -q "option go_package" "$file" && grep -H -o -c "option go_package.*$GO_MOD_PACKAGE/api" "$file" | grep -q ':0$'; then
      buf generate --template buf.gen.gogo.yaml $file
    fi
  done
done

echo "Generating pulsar proto code"
buf generate --template buf.gen.pulsar.yaml

cd ..

# Move gogo generated files from full module path to repo root
cp -r $GO_MOD_PACKAGE/* ./
rm -rf github.com

# Move the pulsar output into api/. The plugins stage into .protogen rather
# than writing to the repo root directly: with paths=source_relative the
# license namespace would land on ./license, which collides with the repo's
# LICENSE file on case-insensitive filesystems. Staging also means the set of
# namespaces never has to be derived — proto packages that are shared
# libraries rather than modules have no module/v1/module.proto to key off.
rm -rf api && mv .protogen api

# Fix incorrect SDK type references for pulsar generated files
find api -type f -name '*.go' -exec sed -i'' -e 's|types "github.com/cosmos/cosmos-sdk/types"|types "cosmossdk.io/api/cosmos/base/v1beta1"|g' {} \;
find api -type f -name '*.go' -exec sed -i'' -e 's|types1 "github.com/cosmos/cosmos-sdk/x/bank/types"|types1 "cosmossdk.io/api/cosmos/bank/v1beta1"|g' {} \;
