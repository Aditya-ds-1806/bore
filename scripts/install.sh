#!/usr/bin/env bash

set -e

version=$1

if [ -z "$version" ]; then
    latest_release_info=$(curl -s https://api.github.com/repos/Aditya-ds-1806/bore/releases/latest)
    latest_version=$(echo "$latest_release_info" | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p')
    version=${latest_version#v}
fi

echo "Installing bore version $version"

arch=$(uname -m)

if [ "$arch" = "x86_64" ]; then
    arch="amd64"
elif [ "$arch" = "aarch64" ] || [ "$arch" = "arm64" ]; then
    arch="arm64"
elif [ "$arch" = "armv6l" ]; then
  arch="armv6"
else
    echo "Unsupported architecture: $arch"
    exit 1
fi

download_url="https://github.com/Aditya-ds-1806/bore/releases/download/v$version/bore_${version}_linux_${arch}.tar.gz"
curl -fsSL -o bore.tar.gz $download_url

mkdir -p ./bore
tar -xzf bore.tar.gz -C ./bore

installDir="$HOME/.local/bin"

mkdir -p $installDir
cp ./bore/bore $installDir/bore
chmod +x $installDir/bore

rm -rf bore.tar.gz ./bore

echo "bore $version installed in $installDir/bore!"
echo "Make sure $installDir is in your PATH."
echo "Run 'bore --help' to get started."
