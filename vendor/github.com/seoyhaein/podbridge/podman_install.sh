#!/bin/bash

set -ex

# install dependency
# Debian, Ubuntu, and related distributions: -> https://podman.io/getting-started/installation
sudo apt-get update
sudo apt-get install \
  gcc \
  btrfs-progs \
  go-md2man \
  iptables \
  libassuan-dev \
  libbtrfs-dev \
  libc6-dev \
  libdevmapper-dev \
  libglib2.0-dev \
  libgpgme-dev \
  libgpg-error-dev \
  libprotobuf-dev \
  libprotobuf-c-dev \
  libseccomp-dev \
  libselinux1-dev \
  libsystemd-dev \
  pkg-config \
  uidmap

# install runc
pushd ~/gopath/src/github.com/seoyhaein/
git clone -b v1.1.3 https://github.com/opencontainers/runc.git && pushd runc
make
sudo cp runc /usr/local/bin
popd
popd

# install conmon
pushd ~/gopath/src/github.com/seoyhaein/
git clone -b v2.1.3 https://github.com/containers/conmon.git && pushd conmon
make
pushd bin/
sudo cp conmon /usr/local/bin
popd
popd
popd

# install podman
pushd ~/gopath/src/github.com/seoyhaein/
git clone -b v4.1.0 https://github.com/containers/podman.git && pushd podman
make binaries
pushd bin/
sudo cp podman /usr/local/bin
popd
popd
popd
sudo systemctl --user enable --now podman.socket && sudo systemctl start --user podman.socket
podman system service -t 0 &

cd ~/gopath/src/github.com/seoyhaein/podbridge