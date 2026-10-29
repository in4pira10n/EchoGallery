# 本地目录更新

设置 → 版本更新支持填写服务器上的更新目录和多个分发目录。分发目录每行一个，递归发现当前系统、架构的 EchoGallery 程序；不会执行发现的程序。Windows 名称为 `EchoGallery.exe`，macOS / Linux 为 `EchoGallery`。备份、缓存、构建目录和符号链接会跳过。手动添加支持目录外的程序，移除后不会被下一次扫描自动加回，可通过“恢复已移除目标”恢复。

实例名称和头像读取该实例旁的 `config.json`，头像优先使用当前同名账户星标资源库，未设置时回退到活动资源库。头像来自资源库 `.echogallery/logo.png`，无头像时使用现有占位图。读取过程中不迁移配置、不创建资源库数据。

`local-update.toml` 示例：

```toml
distribute = ['D:/Apps', 'E:/EchoGallery']
include = ['F:/OtherGallery/EchoGallery.exe']
exclude = ['D:/Apps/Old/EchoGallery.exe']
update_package_dir = 'D:/EchoGalleryUpdates'
update_package_path = ''
```

更新目录本身或其直接子目录可以是解压后的发行目录，也可以包含 ZIP / tar.gz / tgz 包。每个自动选择的发行包须在根目录包含 `manifest.json` 和一对主程序、更新器。manifest 包含语义版本、构建编号、GOOS、GOARCH 和两份 SHA256。系统先选匹配平台的最新稳定语义版本，同版本再按构建编号排序，忽略预发布版。旧 manifest 没有构建编号时仍可读取，但会排在带编号的同版本构建之前。文件名不影响选择。检查时验证程序来源、平台和校验值；发现损坏包时停止并报告错误。

旧配置的 `update_package_path` 仍固定到指定的文件或解压目录，可兼容无 manifest 的旧包；主程序和更新器仍须为当前平台的 EchoGallery 程序。`update_package_dir` 与 `update_package_path` 不能同时设置。界面填写更新目录会清除旧固定路径。

已知版本与构建编号不会降级：语义版本相同的情况下也会比较内部构建编号。若目标程序或更新包没有构建编号，则退回文件校验值比较，无法保证同版本构建的先后顺序。可用实例列表显示各 Gallery 当前构建编号，检查后同时显示所选包的构建编号。检查结果绑定配置、所选包和目标程序/更新器；更新前有变动时须重新检查。各目标在实际替换前再次检查文件内容。更新会重启当前服务；其他正在运行的实例须自行停止以避免文件占用。

GitHub 来源保留在界面，当前明确显示暂未开放，不会回退为本地更新。

资源库设置中的日期、最近扫描时间、容量、照片数、视频数和不支持媒体数会写入应用目录旁的 `library_metrics.json`。进入当前资源库时只重新读取该资源库的 `.echogallery/metadata.sqlite` 并更新对应条目，其他资源库直接使用 JSON 缓存；批量任务完成后也会刷新已完成资源库的条目。

## 构建发行目录

```sh
# 只构建两个程序并写 manifest，不创建压缩包、不上传。
sh scripts/build.sh windows amd64 v3.0.0

# 仅在明确需要压缩包时传入 --package。
sh scripts/build.sh windows amd64 v3.0.0 --package
```

产物保留在 `.build/<GOOS>-<GOARCH>/`。默认版本为 `v3.0.0`，可通过参数指定其他语义版本。手工流水线可使用 `go build -ldflags='-X echogallery/internal/update.Version=v3.0.0'` 注入版本（两个程序都需要），随后运行：

```sh
go run ./cmd/echogallery-manifest -dir <发行目录> -version v3.0.0 -os windows -arch amd64
```

压缩包须包含主程序、更新器和 manifest，放入固定更新目录即可，无须再次更改设置中的版本文件名。

## 版本信息

版本名内置于 `internal/update/catalog.go` 的 `VersionName`，当前为 GitHub Release 标题 `EchoGallery v3.0.0 : Nice to Meet You`。发布新版本时更新此值；运行时不请求 GitHub。

`Version` 保存用于更新比较的语义版本，`VersionCode` 为独立构建编号。构建脚本按北京时间生成 `v3.0.0buildYYYYMMDDHHmm` 并注入主程序及更新器；manifest 工具从主程序读取编号并写入 manifest，同时检查两个程序的编号一致。同一次构建使用同一个编号，压缩包文件名也包含该编号，界面分开展示版本名与版本编号。
