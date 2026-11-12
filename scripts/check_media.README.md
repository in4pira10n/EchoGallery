# EchoGallery 媒体兼容性检查 / Media compatibility audit

## 中文

`check_media.py` 是独立工具，仅使用 Python 标准库。需要 Python 3.10+、FFprobe；实际转换还需要含 `libx264` 和 AAC 编码器的 FFmpeg。Windows 安装后应能运行 `py -3 --version`、`ffprobe -version`、`ffmpeg -version`，也可以通过参数指定 exe 完整路径。

在脚本所在目录打开 PowerShell，检查指定目录及全部子目录：

```powershell
py -3 .\check_media.py "D:\Pictures" --dry-run
```

检查 SMB 目录并把问题写入新 JSON 报告（报告文件必须尚不存在，父目录需要已经存在）：

```powershell
py -3 .\check_media.py "\\192.168.1.4\Pictures" --report "D:\eg-media-report.json"
```

预演转换，检查列出的文件和目标路径；DRY-RUN 不创建目录、转换文件或报告：

```powershell
py -3 .\check_media.py "D:\Pictures" --convert --output "E:\EG-Compatible" --dry-run
```

执行转换，保留源文件和相对子目录，已有目标跳过：

```powershell
py -3 .\check_media.py "D:\Pictures" --convert --output "E:\EG-Compatible" --report "D:\eg-conversion-report.json"
```

FFmpeg 没有加入 PATH 时：

```powershell
py -3 .\check_media.py "D:\Pictures" --ffprobe "C:\Tools\ffmpeg\bin\ffprobe.exe" --ffmpeg "C:\Tools\ffmpeg\bin\ffmpeg.exe" --convert --output "E:\EG-Compatible"
```

- 区分 EG 未收录扩展名、浏览器编码／容器兼容性风险和检查失败。`video_codec` 等是保守兼容性建议，不表示所有浏览器都无法播放；不进行完整解码校验。
- 默认发现常见图片、视频、音频和 RAW 扩展名；`--probe-all` 对其他非已知图片也调用 FFprobe，适合无扩展名／错误扩展名文件，但非媒体会产生检查错误，扫描更慢。
- HEIC/HEIF 是 EG 已支持格式，不要求额外转换。已支持图片仅检查扩展名，不验证损坏、EXIF 或 EG 本机 FFmpeg 的解码能力。
- 视频另存为 H.264 8-bit 4:2:0 + AAC 的 MP4，保留所有音轨。视频名例如 `clip.avi.eg-compatible.mp4`，不同来源扩展名不会互相覆盖。字幕、附件和其他视频流不复制，文件系统日期及完整源元数据不保证保留；HDR 不进行专门的色调映射。浏览器具体能力仍与设备／版本有关。
- 未支持图片默认只报告。明确接受单帧转换和元数据／色彩损失后，可在 `--convert` 之外加 `--convert-images` 转为 PNG。RAW 可能只能解码内嵌预览，不能把输出视为原始全尺寸。
- 输出目录不能是扫描根目录或其上级。放在根目录下时自动排除它；同样排除 `.echogallery`、`.git` 等缓存和符号链接／目录重解析点，避免循环和重复扫描。SMB 必须已经可访问，本工具不挂载共享、不管理凭据。
- 发现阶段显示目录、文件、候选数和当前位置；检查阶段显示序号／总数；FFprobe 超过两秒显示等待位置；转换显示媒体时间、百分比及速度。Windows／SMB 底层目录读取发生阻塞时，这类系统调用无法保证即时进度或即时取消。
- `--probe-timeout 60`、`--convert-timeout 7200` 分别控制单文件检查／转换超时；`Ctrl+C` 取消并停止当前转换子进程、清理未完成的临时输出。已完成输出保留。
- JSON 使用稳定的原因代码，并包含源路径、编码信息、错误及转换结果；检查失败不会自动尝试转换。`existing_unverified` 表示已有输出未被覆盖，也未验证其内容。
- 退出码：`0` 无已知问题，`1` 发现兼容性问题（即使副本已经转换成功），`2` 扫描／检查／转换错误，`130` 用户取消。扫描后会重新列出源文件问题，因为源文件始终没有被修改。
- `--lang en-US` 切换英文；`--help` 查看所有参数。

## English

Standalone Python 3.10+ tool using only the standard library. Install FFprobe; conversion also requires FFmpeg with `libx264` and AAC encoders. Run the PowerShell commands above from the script directory; add `--lang en-US` for English messages. Tool paths may be supplied with `--ffprobe` and `--ffmpeg`.

The recursive audit distinguishes unindexed extensions, conservative browser codec/container risks, and inspection errors. It does not fully decode files or guarantee playback on every browser. Supported images, including HEIC/HEIF, are checked by extension only. Common media and RAW extensions are discovered by default; `--probe-all` additionally probes other non-known images, including extensionless files, but non-media files then produce inspection errors.

`--dry-run` reads directories and runs FFprobe but writes nothing, even with `--report`. Conversion requires both `--convert` and `--output`. Originals are preserved; relative directories and original extensions remain in output names. Existing outputs are skipped without validation. Output may be inside the source tree (automatically excluded), but cannot equal or contain the source root. EG caches, symbolic links and directory reparse points are excluded. UNC shares must already be accessible; the tool does not mount shares or manage credentials.

Videos become H.264 8-bit 4:2:0 MP4 with all audio streams converted to AAC. Subtitles, attachments and additional video streams are not copied. Full metadata, filesystem timestamps and HDR tone mapping are not guaranteed. Unsupported images are report-only unless `--convert-images` is also supplied; PNG output is single-frame and may lose animation, metadata or color information. RAW output may be an embedded preview rather than full resolution.

Discovery reports counts and paths; inspection reports file index/total and slow-probe heartbeats; conversion reports percentage, media time and speed. Per-file timeouts default to 60 seconds for probing and 7200 seconds for conversion. Ctrl+C stops the active conversion and removes incomplete temporary output; completed copies remain. Blocking Windows/SMB filesystem calls cannot promise immediate cancellation or progress.

`--report` creates a new UTF-8 JSON file containing stable reason codes, stream details, errors and conversion results. Its parent directory must exist; existing reports are never overwritten. Inspection failures are not converted automatically. Exit codes: 0 = no known issues, 1 = compatibility issues found (including successfully converted originals), 2 = scan/probe/conversion errors, 130 = cancelled.
