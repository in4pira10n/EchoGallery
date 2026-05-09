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

```text
http://127.0.0.1:8080
```

## TO-DOs

- [x]  `Doc` 示例项目和截图
- [ ]  `Future Plan` `Doc` 视频演示
- [ ]  `ICON` App Icon
- [x] `Doc` `Enhancement` 更改切换 Tab 的快捷键 `Alt` `数字键` 为直接按 `数字键`；并随之更改 README 的快捷键指南
- [ ]  `Fix` 手机端“快进 / 快退”（敲击两下屏幕边缘 1/4）手势会和“暂停 / 播放”（轻点屏幕一下）手势冲突
- [ ]  `Bug` 手机端点击照片并不是直接进入大图浏览；而是先“锁定照片”（出现复选框与收藏 Tag），再点一下才进入浏览页
- [x]  `New Feature` 「相册」详情页现支持按：媒体名称 / 文件大小 / 时间线 / 时间线倒序排序
- [x]  `New Feature` `Video Playback` 视频播放：若视频时长不少于 10 分钟（在「设置」中新建项，可由用户修改），则将视频时常等分为 10「小节」；每一个「小节」要在进度条有一个小小点（比「书签」的圆点小，要与主色呼应）；按 `左右方向键` ：`←` 可前往上一「小节」；`→` 可前往下一「小节」
- [x]  `New Feature` `Video Playback` 在「设置」中新增：视频播放自动播放下一个
- [x]  `Enhancement` 将非相册、设置页的 content 左右 padding 改为 0，并默认照片圆角 2px、图像间距 2px
- [x]  `Enhancement` 删除「保存并重启」；检测用户动了哪些设置，若需要重启则点击「保存」自动重启
- [ ]  `New Feature` 多用户 / 访客用户支持
    - [ ]  `Guest` 访客用户支持
    - [ ]  `Customization` 每个用户应有自己的头像；在注册时应让用户选择头像，并保存在 echogallery-data 里
    - [x]  `Config` 每个用户使用独立 Profile（`echogallery-data/profiles/{user}/profile.json`），包含单独的资源库、缩略图、回收站和应用偏好；用户名仍作为唯一标识
- [ ]  `AI` `Future Plan` 机器学习相关
    - [ ]  `Person` 「人物」相册：内置到「相册」Tab，作为下方 filter 的一项（注意新建空白 SVG）
        - [ ] `Core Service` 初次点击时，应显示需要扫描并发现人物；可参考「相册空白页」或「照片空白页」等等，注意新建空白 SVG；留下按钮让用户「开始扫描」「取消」，显示进度，可随时取消；扫描过程中 App 不可用（参考“构建资源库”页面）；应使用 GPU / MPS 等方式加速
        - [ ] `Core Service` `UI` 扫描完成后展示人物相册，样式参考相册页；其本质也按照「相册」来划分，只是划分相册的标准按照「人物」
    - [ ]  `Future Plan` `AI` 相似照片识别
- [x] `UI` UI Redesign : Compact & Glass → v2.0.0
   - [x] `Sidebar` 左侧侧边栏「减负」：移「项目仓库」「模式切换」「退出登录」「退出程序」至设置页；删除「折叠侧边栏」，将侧边栏默认折叠
   - [x] `Menubar` 顶栏 Redesign：移除「搜索框」；在页面右下角新增 44px 圆形玻璃「搜索」按钮
   - [x] `Core Elements` 整体采用更强调「沉浸感」的悬浮设计：将侧边栏 #main-nav、顶栏 .topbar 采用悬浮设计，圆角 999px，背景模糊；头像单独以圆形放置
- [x]  `Fix` `UI` 隐藏滚动条；播放视频时不显示系统播放控制
- [x]  `Bug` `Performance` 停止一个操作时并不是「真正停止」：比如点击加载全部时间线后，点击取消，相关线程还在运行，点击图片还需要加载；如果刷新则可解决问题；改为停止操作后立刻终止相关线程
- [x] `Doc` 更新介绍图像
- [x] `Fix` 时间线现可记住「倒序」「正序」
- [x] `New Feature` 显示 EXIF（如果有），其中缺少的 SVG 不要复用现有的，要新建
- [x] `Bug` Toggle “个人收藏”后视频会自动重头播放
- [x] `Bug` “在时间线查找”取消按钮不生效
- [x] `Bug` WMV 视频仍不支持播放
- [x] `Fix` `UI` 网页 favicon 添加圆角
- [x] `Fix` `UI` 灯箱的大图不再需要边距，直接紧靠窗口边缘
- [x] `New Feature` 顶部栏：添加并支持筛选（以图标形式展示）：图片 / 视频
- [x] `New Feature` 视频播放书签
- [x] `Fix` `Windows` 视频播放不再默认暂停
- [x] `Fix` `UI` “收藏”和“取消收藏”应使用不同的 SVG
- [x] `UI` Big UI Redesign → v1.0.2
  - [x] `Fix` `UI` 上传卡片的 modal-title SVG 与文字应该在同一行并居中
  - [x] `Fix` `UI` 视频的音量控制给出视觉反馈（底部音量条）
  - [x] `Enhancement` `UI` 时间线 Tab 上方添加一个“三个横线” Tab（仅点击切换）：将 Tab 宽度缩为仅显示 SVG，此时资源库仅显示头像
  - [x] `Enhancement` `UI` 灯箱顶栏右上角“下载”“收藏”“分享”“播放”；左上角“返回”；“适应”：重构为仅显示 SVG
  - [x] `Enhancement` `UI` 灯箱底部栏字段名（如类型、MIME 等标题文字）重构为仅显示 SVG
  - [x] `Enhancement` `UI` 同时检查其他顶栏多余文字（如“上一个相册”、“下一个相册”等）重构为仅显示 SVG；注意所有的 SVG 暴露在 web static SVG 里
  - [x] `Enhancement` `UI` 灯箱顶栏右上角“分享”后添加一个省略号按钮（SVG），用于不方便点击右键的设备使用右键菜单：其功能为弹出一个右键菜单（弹出来源于灯箱右上方）
- [x] `Fix` “适应高度”改为“适应”：高度和宽度哪一个更小就适应哪一个值（否则在手机的显示会很奇怪）
- [x] `Fix` `UI` 缩放为大图之后左右的 prev、next 也会跟着移动；应该固定位置
- [x] `Bug` 错误: 重启 EchoGallery 失败: not supported by windows
- [x] `Fix` 优化上传功能，包含：SVG 尺寸不正常；图像过大（BMP），改为原格式上传；上传路径不能是本地缓存数据库，改为在目标资源库新建一个文件夹将上传的文件放入其中（如果用户没有进行上传操作则不新建，尽量不破坏原目录结构）等
- [x] `Fix` `UI` 上传卡片（upload-zone）的 SVG 尺寸太大
- [x] `New Feature` "回忆"（现在是历史记录功能） Tab
- [x] `Fix` 相册 Tab： `Q` `E` 切换上 / 下一个相册
- [x] `Future Plan` 自动检查和安装更新（设置）：自动检测 GitHub Release，若有更新的版本号提示用户安装
- [x] `Enhancement` 高亮边框现常驻，可用 WASD 选择照片，并和主色呼应
- [x] `Enhancement` “相册” Tab 也要和其他 Tab 一样“记得”上次浏览位置
- [x] `Enhancement` 右键菜单左侧添加图标指示，暴露在 web static SVG 文件夹下
- [x] `New Feature` 在当前浏览相册顶部显示“上一个相册”“下一个相册”
- [x] `New Feature` 搜索和筛选结果支持打包下载全部
- [x] `Future Plan` 全局搜索
- [x] `Enhancement` 去掉灯箱顶部、底部栏的黑色遮罩，将圆角矩形元素使用半透明 + 背景模糊处理，加强照片大图浏览沉浸感
- [x] `New Feature` 按住 `Ctrl`（Windows）或 `Command`（Mac）会将缩放等级暂时调满并聚焦鼠标当前区域
- [x] `Fix` 隐藏侧栏后不需要留出一小部分侧栏
- [x] `Enhancement` 将“扫描与构建资源库”页面也显示在前端并使用进度条同步状态以及 ETA
- [x] `Enhancement` 支持“扫描与构建资源库”后退出应用；适合无人值守的较大资源库数据库构建
- [x] `Fix` user-select return none（防止意外框选内容影响体验）
- [x] `Fix` 修改复选框处理区域的位置为顶部中间（同“时间线”“缩放”等内容）
- [x] `Fix` 更改并重新排布底部信息从左到右为：类型、MIME、拍摄时间、尺寸、宽高比（近似）、大小（MB）
- [x] `Fix` 更改原先相册按钮的排布从左到右为：“返回相册”“下载相册”“删除相册（使用红色提醒）”
- [x] `New Feature` 增加更多格式支持
- [x] `New Feature` 图片支持“在相册中查看”：如果多个相册存在则在右键菜单中提供多个相册跳转的选项
- [x] `Fix` “相册” Tab：如果相册为文件夹，要使用一种颜色变体标识（注意和主色的呼应）；如果是用户创建的相册则使用另一种颜色变体
- [x] `Fix` “相册” Tab：列表模式展示下，要根据设备尺寸响应式调整列数；而不是一行只展示 1 个相册
- [x] `Fix` “相册” Tab：打开某个相册后按 Esc，可以从这一个相册跳转至上一层（全部相册）
- [x] `Fix` 优化显示逻辑，优先保证目前所在页面图像加载（比如从下方位置滚动时间线到上方，中间的图像可以先不加载）
- [x] `Fix` 若未加入“个人收藏”，则心型标志不默认在缩略图显示（类似于勾选框）；只显示已收藏图像的实心红心
- [x] `Fix` 全局图片墙缩放级别也记录进 config.json 中，并添加更小的缩放级别（一页展示更多照片）
- [x] `Fix` 大图模式：重新设计和排版顶部栏元素，注意和底部栏以及整体元素设计风格呼应
- [x] `Enhancement` 寻找更高效（构建速度更快、体积更小、从磁盘加载速度更快）的缩略图格式并使用新格式重构数据库
- [x] `Enhancement` 在“时间线” Tab 添加立即加载全部时间线图片功能的按钮：显示一个 popup，立即（以更快速度）加载所有时间线图片（这期间整个 App 功能不可用，优先以全部资源加载时间线和缩略图；防止后续浏览的时候被打断）；同时展示进度条；可提供“取消”按钮打断操作
- [x] `Enhancement` 在“乱序相册” Tab 添加立即加载全部媒体功能，并批量预加载缩略图
- [x] `Bug` 某些 PNG 图像缩略图显示失败 / 缺失
- [x] `Bug` 资料库图像无法上传
- [x] `Bug` 资料库图像无法更改
- [x] `Bug` 大图缩放后无法移动查看头部
- [x] `Bug` 浏览时间线媒体返回时，总是从最顶部滚动下来（从“乱序相册”定位到时间线时也是如此）
- [x] `Bug` “乱序相册”按 Esc 退出时无论如何都返回时间线而不是原位置
- [x] `Bug` `Windows` 打开正常但提示打开失败: 在文件管理器中打开失败: exit status 1
- [x] `Bug` 扫描资料库时跳过失败项
- [x] `Bug` `Windows` 在 Finder 中打开等 macOS 遗留操作在 Windows 下不生效
- [x] `Bug` “设置”页面的资源库头像与实际不同步
- [x] `Doc` 补充：快捷键指南
- [x] `Enhancement` `URGENT` CPU / 内存资源占用过高
- [x] `Enhancement` 大图模式下方信息显示限制 1 行，多余信息改为滚动（保证图片显示）
- [x] `Fix` 移除默认资料库图像，改为随机抓取一张
- [x] `Fix` 资料库图像逻辑：给定图像 -> 按中心裁剪为正方形 -> 在左上角显示为圆形，上传图像时也要提示用户裁剪
- [x] `Fix` 调整缩略图尺寸后重新生成缩略图
- [x] `Fix` “乱序相册”每次打开都要重新加载
- [x] `Fix` “在时间线中查看”精准跳转并高亮显示 1 秒
- [x] `Fix` 按 Esc 退出时默认执行“在时间线中查看”的逻辑，即精准跳转
- [x] `Fix` 「个人收藏」显示总相片数
- [x] `Fix` 移除鼠标 hover 的缩放效果
- [x] `Fix` “缩放”新增“适应”选项，按 `Alt + 0` 返回“适应”
- [x] `Fix` 修正大图模式“适应高度”实际高度控制不准的问题
- [x] `Fix` `External HDD` 加载所有时间线媒体时应暂时禁止用户操作并展示进度，待完全加载完成再开放操作；同时优化加载速度
- [x] `Fix` 分享链接可在设置中管理
- [x] `Fix` 「在时间线中查看」可在任何地方的右键菜单可用
- [x] `Fix` 按 `F` 可快捷加入 / 移除个人收藏
- [x] `Fix` 大图模式在加载中时，也要有转圈的提示
- [x] `Fix` 修正「资源库头像」：上传之后弹出一个 popup 提示用户进行编辑（裁剪为正方形等）
- [x] `New Feature` 相册支持列表 / 大图显示切换
- [x] `New Feature` 时间线倒序
- [x] `New Feature` 个人收藏支持全选下载
- [x] `New Feature` 在大图模式下方添加更多的照片 / 视频文件信息
- [x] `New Feature` 点击资源库头像跳转设置

## Thanks

感谢以下开源项目：

- [gin-gonic/gin](https://github.com/gin-gonic/gin)
- [modernc/sqlite](https://github.com/modernc/sqlite)
- [rwcarlsen/goexif](https://github.com/rwcarlsen/goexif)
- [DoYoungDo/PhotoAlbum](https://github.com/DoYoungDo/PhotoAlbum)
- 以及本项目依赖的其它开源库
