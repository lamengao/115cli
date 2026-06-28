# 115cli

`115cli` is a Go CLI for 115.com cloud drive using the official 115 Open API.

## Disclaimer

`115cli` is not an official 115 product and is provided only for personal learning purposes. Use it at your own risk; you are solely responsible for any issues, losses, account problems, or other consequences arising from its use.

## Build

```sh
go build -o ~/bin/115cli .
```

Project convention: build the global `115cli` executable into `~/bin/115cli`.

## Auth

Cookie auth is the recommended path when you cannot get a 115 Open Platform App ID.

```sh
./115cli auth cookie 'UID=...; CID=...; SEID=...; KID=...'
```

You can copy the cookie from a browser session that is already logged in to `115.com`.
Keep it private: this cookie grants access to your account.

If you do have a 115 Open Platform App ID, the Open API login flow is still available:

```sh
./115cli auth login --client-id '<your-115-open-platform-app-id>'
```

If you already have a refresh token, you can still save it manually:

```sh
./115cli auth --refresh-token '<refresh-token>'
```

The default config path is `~/.config/115cli/config.json`. You can override it with `--config`.

## Commands

```sh
./115cli ls /
./115cli info /remote/folder
./115cli space
./115cli sha1 /remote/file.txt
./115cli down /remote/file.txt ./file.txt
./115cli down /remote/folder ./downloads
./115cli mkdir /remote/folder
./115cli up ./local-file.txt /remote/folder
./115cli up ./local-folder /remote/backup
./115cli sync ./local-folder /remote/backup
./115cli sync --delete-remote-missing ./local-folder /remote/backup
./115cli del /remote/file-or-folder
./115cli download quota
./115cli download list
./115cli download list --filter completed
./115cli download add "https://example.com/file.mp4"
./115cli download delete <info_hash>
./115cli download status <info_hash>
./115cli download retry <info_hash>
./115cli download clear --filter failed
```

Remote paths use Unix-style syntax. The root folder ID defaults to `0`; use `auth --root-id <id>` to pin another root.

`sync` compares the local directory with the remote directory and uploads files or directories that are present locally but missing remotely. If a same-name remote file has a different size, `sync` deletes the remote file first and uploads the local file again. It does not delete or modify local files.
Symlinked files and directories are followed during `sync`; directory symlink loops are skipped.
Use `--delete-remote-missing` to delete remote files that do not have a same-name local entry. Remote directories are not deleted by this option.
Every remote change made by `sync` is printed, including uploads, directory creation, and deletes.

Note: with cookie authentication, `ls`, `info`, `space`, `sha1`, `down`, `up`, `sync`, `del`, and `download` use 115 web endpoints. Cookie uploads first try instant upload by SHA1 and fall back to OSS upload with 115 web upload signing. Creating cloud download tasks may trigger a captcha challenge; the CLI reports the 115 API error if that happens.
