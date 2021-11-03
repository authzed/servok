# Servok

[![Container Image](https://img.shields.io/github/v/release/authzed/servok?color=%232496ED&label=container&logo=docker "Container Image")](https://quay.io/repository/authzed/servok?tab=tags)
[![Docs](https://img.shields.io/badge/docs-authzed.com-%234B4B6C "Authzed Documentation")](https://docs.authzed.com)
[![GoDoc](https://godoc.org/github.com/authzed/servok?status.svg "Go documentation")](https://godoc.org/github.com/authzed/servok)
[![Build Status](https://github.com/authzed/servok/workflows/Build%20&%20Test/badge.svg "GitHub Actions")](https://github.com/authzed/servok/actions)
[![Discord Server](https://img.shields.io/discord/844600078504951838?color=7289da&logo=discord "Discord Server")](https://discord.gg/jTysUaxXzM)
[![Twitter](https://img.shields.io/twitter/follow/authzed?color=%23179CF0&logo=twitter&style=flat-square "@authzed on Twitter")](https://twitter.com/authzed)

Servok is a service that provides endpoint metadata for client side load balancing.

See [CONTRIBUTING.md] for instructions on how to contribute and perform common tasks like building the project and running tests.

[CONTRIBUTING.md]: CONTRIBUTING.md

## Getting Started

### Running Servok locally

```sh
servok --grpc-no-tls
```

Run `servok -h` for all config options.
