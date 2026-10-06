# EchoGallery

The next generation local photo gallery managing app.

<img src="https://github.com/user-attachments/assets/dae16cf6-55c7-4c46-be5c-136dfb5ebd53" />

## Intro

EchoGallery 是一个本地相册管理 Web 应用，适合想在自己电脑或局域网内浏览、管理和分享照片/视频的用户。

它的定位包括：

- 需要一个轻量级、本地运行的媒体库
- 想避免依赖云服务，保护隐私
- 想把本地照片/视频按“时间线 + 相册 + 收藏 + 回收站”方式管理
- 需要简单的分享链接功能，让家人或朋友访问部分照片

## Features

- 时间线浏览
- 大图浏览模式
- 乱序相册
- 幻灯片顺序 / 随机播放
- 完整的链接式分享功能
- 本地资源库管理
- 相册管理
- 收藏与垃圾箱
- 视频缩略图刷新

## Supported Formats

- 图片：JPG / JPEG、PNG、GIF、WebP、BMP、TIFF
- 视频：MP4、M4V、MOV、WebM、MKV、AVI、TS / MTS / M2TS、MPG / MPEG、3GP / 3G2、OGV

> 视频封面生成依赖 `ffmpeg`。如果未安装 `ffmpeg`，视频文件仍可管理，但视频缩略图可能无法生成。

## Keyboard Shortcuts

### 全局快捷键

- `Alt + 1`: 切换到时间线视图
- `Alt + 2`: 切换到收藏视图
- `Alt + 3`: 切换到乱序相册视图
- `Alt + 4`: 切换到相册视图
- `Alt + 5`: 切换到回忆视图
- `Alt + 6`: 切换到垃圾箱视图
- `Alt + 7`: 切换到设置视图

### 大图浏览模式

- `Esc`: 关闭大图模式
- `A`: 上一张
- `D`: 下一张
- `Space`: 切换幻灯片播放
- `F`: 加入 / 移除个人收藏
- `Alt + 0`: 返回“适应”

### 视频播放器快捷键 (使用系统播放器时)

- `X`: 暂停/播放
- `Z`: 后退 5 秒
- `C`: 前进 5 秒
- `W`: 音量 +
- `S`: 音量 -
- `M`: 静音
- `F`: 全屏
- `Esc`: 退出全屏
- 更多快捷键请参考 `ZXCWASD.conf` 文件

## Quick Start

_请确保您的电脑安装了 `ffmpeg`，否则视频缩略图无法显示！_

1. 下载 Release
   - 直接从 GitHub Release 页面下载对应平台的压缩包或可执行文件。
2. 解压并运行
   - macOS / Linux: 解压后在终端进入目录，执行 `./echogallery`
   - Windows: 直接双击 `echogallery.exe` 或在命令提示符中运行 `echogallery.exe`
3. 打开浏览器
   - 访问 `http://127.0.0.1:8080`，打开 EchoGallery 初始化页面。
4. 完成首次设置
   - 按页面提示创建管理员账户。
   - 选择或新建一个“资源库文件夹”，用于存放照片和应用数据。
5. 导入资源库
   - 在设置里选择图片 / 视频所在的文件夹作为资源库。
   - 等待应用扫描并导入现有媒体。
6. 开始浏览
   - 进入时间线、大图浏览、相册、收藏等页面查看内容。

> 第一次使用时，如果已经有 `config.json` 和数据库，EchoGallery 会直接进入主界面；否则会自动进入设置向导。

## Installation

### Requirements

- Go 1.26+
- ffmpeg

### Build

```bash
git clone https://github.com/in4pira10n/EchoGallery.git
cd EchoGallery
go build -o echogallery .
```

### Run

```bash
./echogallery
```

第一次运行时，如果没有 `config.json`，会自动启动初始化设置页面，通常访问：

如果是从另一台电脑迁移，可直接在初始化页选择“导入换机配置包”。配置包会恢复账户、权限、个人偏好、头像、品牌资源与快捷键；资源库本身请连同隐藏的 `.echogallery` 目录完整复制。若磁盘路径变化，重启后在资源库恢复页选择新路径即可。

资源库的数据库、缩略图、播放缓存、回收站、Logo、名称、强调色与稳定 ID 均保存在 `<资源库>/.echogallery/`。设置页“应用配置”可导出换机配置包；运行时锁、页面会话、旧缓存、旧电脑的全局缓存路径和本地更新路径不会打包。配置包包含账户与密码哈希，应当像密码备份一样妥善保管。

```text
http://127.0.0.1:8080
```

## Thanks

感谢以下开源项目：

- [gin-gonic/gin](https://github.com/gin-gonic/gin)
- [modernc/sqlite](https://github.com/modernc/sqlite)
- [rwcarlsen/goexif](https://github.com/rwcarlsen/goexif)
- [DoYoungDo/PhotoAlbum](https://github.com/DoYoungDo/PhotoAlbum)
- 以及本项目依赖的其它开源库
