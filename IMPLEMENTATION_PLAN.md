# 115.com 网盘 CLI 实现计划

## Summary

从空目录新建一个 Go CLI 项目，二进制名为 `115cli`，核心命令为 `ls`、`down`、`up`，并额外提供 `auth` 保存官方 Open API 的 `refresh_token`。

技术路线锁定为：Go + 115 官方 Open API + 类 Unix 远端路径，例如 `/电影/a.mp4`。参考资料包括 [OpenList 的 115 Open 文档](https://doc.oplist.org/guide/drivers/115_open) 和 [AList 115_open Go driver 公开接口](https://pkg.go.dev/github.com/alist-org/alist/v3/drivers/115_open)。

## Key Changes

- 初始化 Go module，使用 `spf13/cobra` 组织 CLI：
  - `115cli auth --refresh-token <token>`：写入本机配置。
  - `115cli ls <remote-path>`：列目录，默认 `/`。
  - `115cli down <remote-path> [local-path]`：下载文件或目录。
  - `115cli up <local-path> <remote-dir-or-path>`：上传文件或目录。
- 配置文件默认放在用户配置目录，例如 `~/.config/115cli/config.json`，保存：
  - `refresh_token`
  - 自动刷新得到的 `access_token`
  - token 过期时间或最近刷新时间
  - `root_id` 默认 `0`
- 实现 `internal/open115` 客户端：
  - 自动用 `refresh_token` 刷新 `access_token`，遇到 401 类错误重试一次。
  - 支持目录列表、路径解析、创建目录、获取下载链接、上传文件。
  - 远端路径解析通过逐级 `ls` 查找目录名/文件名，根目录映射为 `root_id=0`。
- `ls` 输出：
  - 默认列出名称、类型、大小、修改时间。
  - 目录显示为 `dir`，文件显示为 `file`。
- `down` 行为：
  - 文件下载到指定本地路径；若目标是目录则保持原文件名。
  - 目录递归下载，保留远端目录结构。
  - 支持断点续传：本地 `.part` 文件存在时使用 HTTP Range 继续下载，完成后原子重命名。
  - 默认跳过已完整存在且大小一致的文件。
- `up` 行为：
  - 文件上传到远端目录；若目标路径带文件名则按该文件名上传。
  - 目录递归上传，远端缺失目录自动创建。
  - 优先尝试官方接口可用的秒传/快速上传能力；失败后回退普通上传。
  - 上传进度按文件显示，目录上传遇到单文件失败时继续后续文件，最后汇总失败列表。

## Public Interfaces

- CLI:
  - `115cli auth --refresh-token <token> [--root-id 0]`
  - `115cli ls [remote-path]`
  - `115cli down <remote-path> [local-path]`
  - `115cli up <local-path> <remote-path>`
  - 通用参数：`--config <path>`、`--verbose`
- Go 内部接口：
  - `Client.List(ctx, remotePath string) ([]Entry, error)`
  - `Client.Download(ctx, remotePath, localPath string) error`
  - `Client.Upload(ctx, localPath, remotePath string) error`
  - `Entry` 包含 `ID`、`Name`、`IsDir`、`Size`、`Sha1`、`UpdatedAt`、`DownloadURL` 等字段。

## Test Plan

- 单元测试：
  - 配置读写与 token 刷新状态更新。
  - 类 Unix 远端路径解析：根目录、嵌套目录、文件不存在、重名冲突。
  - 下载路径规划：文件到文件、文件到目录、目录递归。
  - 上传路径规划：文件上传、目录递归、自动创建远端目录。
- HTTP mock 测试：
  - token 过期后刷新并重试。
  - `ls` 分页返回合并。
  - 下载断点续传 Range 请求。
  - 上传秒传成功、秒传失败回退普通上传。
- 本地验证：
  - `go test ./...`
  - `go build ./...`
  - 用 mock server 跑 CLI 集成测试，不依赖真实 115 账号。

## Assumptions

- 使用官方 Open API，不使用浏览器 cookie 接口。
- 第一版不内置完整扫码 OAuth；用户通过外部方式拿到 `refresh_token` 后粘贴给 `auth`。
- 根目录默认是 115 Open API 的 `0`，后续可通过 `--root-id` 指定。
- 不实现删除、移动、分享、离线下载等额外命令。
