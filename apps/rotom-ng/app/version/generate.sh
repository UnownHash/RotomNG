#!/bin/sh

version=`cat version.txt`
cat > version.go <<EOF
package version

// This file is generated from version.txt. Do not edit directly.
// AppVersion is the current application version.
const AppVersion = "$version"
EOF
