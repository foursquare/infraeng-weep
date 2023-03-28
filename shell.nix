{ pkgs ? import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/ee01de29d2f58d56b1be4ae24c24bd91c5380cea.tar.gz") {} }:

with pkgs;

mkShell {
  buildInputs = [
    go_1_17
    upx
    docker
    goreleaser
  ];
}
