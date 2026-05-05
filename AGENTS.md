# 115cli Agent Guide

## Project Overview

- `115cli` is a Go CLI for 115.com cloud drive using the official 115 Open API and, when cookie authentication is configured, 115 web endpoints.
- Remote paths use Unix-style syntax. The root folder ID defaults to `0`; users can pin another root with `auth --root-id <id>`.
- The default config path is `~/.config/115cli/config.json`. Users can override it with `--config`.

## Build And Test

- Build the global executable with `go build -o ~/bin/115cli .`.
- The project convention is to install the binary at `~/bin/115cli`.
- Run tests with `go test ./...`.

## Auth

- Cookie auth is the recommended path when the user cannot get a 115 Open Platform App ID:
  `./115cli auth cookie --cookie 'UID=...; CID=...; SEID=...; KID=...'`
- Treat cookies, refresh tokens, access tokens, and config files as sensitive secrets. Do not print real values, commit them, or include them in logs or examples.
- Open API login is available for users with a 115 Open Platform App ID:
  `./115cli auth login --client-id '<your-115-open-platform-app-id>'`
- Users with an existing refresh token can save it with:
  `./115cli auth --refresh-token '<refresh-token>'`

## Commands

- `./115cli ls /`
- `./115cli down /remote/file.txt ./file.txt`
- `./115cli down /remote/folder ./downloads`
- `./115cli up ./local-file.txt /remote/folder`
- `./115cli up ./local-folder /remote/backup`
- `./115cli sync ./local-folder /remote/backup`
- `./115cli del /remote/file-or-folder`

## Sync Behavior

- `sync` compares the local directory with the remote directory and uploads only files or directories that are present locally but missing remotely.
- `sync` must not delete or modify local files.
- With cookie authentication, `ls`, `down`, `up`, `sync`, and `del` use 115 web endpoints.
- Cookie uploads first try instant upload by SHA1 and fall back to OSS upload with 115 web upload signing.
