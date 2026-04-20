# EchoGallery

A lightweight local photo gallery app.

## TO-DOs

- [ ] `New Feature` 多用户 / 访客用户支持
- [ ] `New Feature` 增加更多格式支持
- [ ] `Bug` 某些 PNG 图像缩略图显示失败 / 缺失
- [ ] `Fix` 调整缩略图尺寸后重新生成缩略图
- [ ] `Fix` “乱序相册”每次打开都要重新加载
- [ ] `Bug` `Windows` 打开正常但提示打开失败: 在文件管理器中打开失败: exit status 1
- [ ] `Enhancement` CPU / 内存资源占用过高
- [ ] `Bug` 扫描资料库时跳过失败项
- [x] `Bug` `Windows` 在 Finder 中打开等 macOS 遗留操作在 Windows 下不生效
- [x] `Bug` “设置”页面的资源库头像与实际不同步
- [x] `New Feature` 在大图模式下方添加更多的照片 / 视频文件信息
- [x] `New Feature` 点击资源库头像跳转设置
- [x] `Fix` 分享链接可在设置中管理

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
  - 多资源库支持
  - 库头像与设置同步
  - 库修复、回收站与恢复机制
- 相册管理
  - 创建相册
  - 添加 / 删除媒体
  - 下载相册内容
- 收藏与垃圾箱
- 视频缩略图刷新
- 系统播放器播放与在文件管理器中定位
- 基于 SQLite 的本地数据库

## Quick Start

*请确保您的电脑安装了 `ffmpeg`，否则视频缩略图无法显示！*

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

```text
http://127.0.0.1:8080
```

## Thanks

感谢以下开源项目：

- [gin-gonic/gin](https://github.com/gin-gonic/gin)
- [modernc/sqlite](https://github.com/modernc/sqlite)
- [rwcarlsen/goexif](https://github.com/rwcarlsen/goexif)
- [PhotoAlbum](https://github.com/DoYoungDo/PhotoAlbum/issues)
- 以及本项目依赖的其它开源库
