#!/usr/bin/env python3
"""递归检查 EchoGallery 媒体兼容性（Python 3.10+，仅标准库）。

Windows PowerShell 示例：
  py -3 scripts/check_media.py 'D:\\Pictures' --dry-run
  py -3 scripts/check_media.py '\\\\NAS\\Pictures' --report 'D:\\eg-report.json'
  py -3 scripts/check_media.py 'D:\\Pictures' --convert --output 'E:\\Converted' --dry-run
  py -3 scripts/check_media.py 'D:\\Pictures' --convert --output 'E:\\Converted'

依赖：ffprobe；转换另外需要包含 libx264 / AAC 编码器的 ffmpeg。
可通过 --ffprobe / --ffmpeg 传入 exe 的完整路径，路径有空格时加引号。

兼容策略：区分 EG 的格式收录能力与浏览器直接播放能力。H.264 8-bit
4:2:0 + AAC 的 MP4 作为保守转换目标；浏览器依赖项目不能据此断言绝对
不可播放。检查编码信息不是完整解码测试，不能保证不存在损坏或音画问题。
HEIC/HEIF 已由 EG 提供映射，不要求用户转换。图片按扩展名检查，不解析
相机 RAW 内容；--probe-all 可额外发现无扩展名或错误扩展名的视频。

所有转换保留源文件，视频保留所有音轨并转 AAC，不烧录／复制字幕。
图片转换必须额外传 --convert-images，输出单帧 PNG，动画、多页、HDR、
RAW 全尺寸和 EXIF/GPS 保留均不保证。默认只报告这些图片。
DRY-RUN 仍读取目录并运行 ffprobe，但绝不转换或创建报告／输出目录。
"""
from __future__ import annotations

import argparse
import json
import mimetypes
import math
import os
from pathlib import Path
import queue
import shutil
import subprocess
import sys
import tempfile
import threading
import time

# 与 internal/media/formats.go 和 internal/image/meta.go 对齐；更新 EG 格式表
# 时应一并核对这里。额外候选格式仅用于发现，不表示 EG 支持。
IMAGES = set('.jpg .jpeg .png .gif .webp .bmp .tif .tiff .heic .heics .heif .heifs .hif'.split())
HEIF = set('.heic .heics .heif .heifs .hif'.split())
VIDEOS = set('.mp4 .m4v .mov .webm .mkv .avi .wmv .wma .ts .mts .m2ts .mpg .mpeg .3gp .3g2 .ogv'.split())
EXTRA_IMAGES = set('.avif .jxl .svg .ico .psd .psb .dng .cr2 .cr3 .nef .nrw .arw .raf .orf .rw2 .pef .srw .raw .exr .hdr .jp2 .jpf .jpx'.split())
EXTRA_VIDEOS = set('.flv .f4v .vob .mxf .m2v .divx .asf .rm .rmvb .mpe .mpv .mjpeg .mjpg .qt .m4a .mp3 .flac .ogg .opus .wav .aac .aiff .aif .ac3 .dts'.split())
SKIP_DIRS = {'.echogallery', '.git', '$recycle.bin', 'system volume information'}

# 命令行工具也遵循项目 i18n：稳定 reason code 用于报告，展示文本独立翻译。
MESSAGES = {
 'zh-CN': {
  'start':'开始扫描：{root}（扫描过程中持续显示计数；总数在遍历结束后确定）',
  'walk':'发现：目录 {dirs}，文件 {files}，媒体候选 {media}，耗时 {elapsed:.1f}s | {path}',
  'probe':'检查 {index}/{total} | 已发现问题 {issues} | {path}',
  'waiting':'{phase}运行中 {elapsed:.0f}s | {path}',
  'progress':'转换 {index}/{total} | {percent} | 媒体时间 {seconds:.1f}s | 速度 {speed} | {path}',
  'plan':'DRY-RUN 转换计划：{source} -> {target}',
  'saved':'已生成：{path}', 'skip':'保留已有文件，跳过：{path}',
  'issue':'[{code}] {reason} | {path}',
  'done':'完成：候选 {total}，无已知问题 {ok}，问题 {issues}，检查错误 {errors}，转换成功 {converted}，转换失败 {failed}，跳过已有 {skipped}，耗时 {elapsed:.1f}s',
  'dry':'DRY-RUN：只读检查，不创建目录、报告或转换文件。',
  'report':'报告已保存：{path}', 'error':'错误：{error}', 'cancel':'已取消；当前子进程已停止，未完成的转换临时文件已清理。',
  'probe_phase':'ffprobe ', 'convert_phase':'FFmpeg ',
  'extension':'EG 未收录此扩展名', 'video_codec':'视频编码／像素格式不在保守浏览器兼容范围',
  'audio_codec':'至少一条音轨不在保守浏览器兼容范围', 'container':'容器需要 EG 转换缓存或依赖浏览器，建议转换为 MP4',
  'no_video':'未发现可播放的视频画面流（音频文件不能作为 EG 视频）',
  'probe_failed':'ffprobe 检查失败；不能据此认定文件已损坏',
  'walk_failed':'目录访问失败，扫描结果不完整', 'image_manual':'图片转换未启用，或该文件没有可转换的画面；需人工检查',
  'tool_missing':'找不到工具：{tool}；请安装或使用 --{tool} 指定完整路径',
  'output_required':'--convert 必须指定 --output；输出目录不能是扫描根目录或其上级',
  'input_invalid':'扫描目录不存在或无法访问：{path}', 'timeout':'子进程超时（{seconds}s）',
  'help':'递归检查 EG 媒体兼容性，可选择安全另存转换。默认只检查。',
  'root_help':'资源库／媒体根目录，支持 Windows UNC 路径', 'convert_help':'转换报告中的可转换文件，保留原文件',
  'output_help':'转换输出根目录，保留相对子目录', 'dry_help':'只读检查并展示转换计划，不写任何文件',
  'report_help':'可选 JSON 报告路径（DRY-RUN 不写入）', 'all_help':'对所有非已知图片运行 ffprobe（慢，可能包含非媒体检查错误）',
  'images_help':'允许未支持图片转单帧 PNG（可能丢失动画、EXIF／GPS 或 HDR）',
  'timeout_help':'单文件检查超时秒数，默认 60', 'ct_help':'单文件转换超时秒数，默认 7200',
  'tool_help':'工具命令或 exe 完整路径', 'lang_help':'提示语言',
 },
 'en-US': {
  'start':'Scanning: {root} (live counts; total becomes known after traversal)',
  'walk':'Discovery: {dirs} directories, {files} files, {media} candidates, {elapsed:.1f}s | {path}',
  'probe':'Inspecting {index}/{total} | {issues} issues so far | {path}',
  'waiting':'{phase}running {elapsed:.0f}s | {path}',
  'progress':'Converting {index}/{total} | {percent} | media {seconds:.1f}s | speed {speed} | {path}',
  'plan':'DRY-RUN conversion: {source} -> {target}', 'saved':'Created: {path}',
  'skip':'Existing file preserved, skipping: {path}', 'issue':'[{code}] {reason} | {path}',
  'done':'Done: {total} candidates, {ok} without known issues, {issues} issues, {errors} inspection errors, {converted} converted, {failed} conversions failed, {skipped} existing skipped, {elapsed:.1f}s',
  'dry':'DRY-RUN: read-only inspection; no output directories, reports or converted files.',
  'report':'Report saved: {path}', 'error':'Error: {error}',
  'cancel':'Cancelled; active subprocess stopped and incomplete conversion temporary files removed.',
  'probe_phase':'ffprobe ', 'convert_phase':'FFmpeg ',
  'extension':'Extension is not indexed by EG', 'video_codec':'Video codec / pixel format is outside the conservative browser compatibility baseline',
  'audio_codec':'At least one audio stream is outside the conservative browser compatibility baseline',
  'container':'Container needs an EG conversion cache or browser-specific support; MP4 conversion recommended',
  'no_video':'No playable video stream (audio-only files cannot be used as EG videos)',
  'probe_failed':'ffprobe inspection failed; this alone does not prove file corruption',
  'walk_failed':'Directory access failed; scan is incomplete', 'image_manual':'Image conversion is disabled, or no convertible picture stream exists; inspect manually',
  'tool_missing':'Tool not found: {tool}; install it or provide its full path with --{tool}',
  'output_required':'--convert requires --output; output must not equal or contain the scan root',
  'input_invalid':'Scan directory is missing or inaccessible: {path}', 'timeout':'Subprocess timed out ({seconds}s)',
  'help':'Recursively audit EG media compatibility; optionally convert into a separate directory. Inspection only by default.',
  'root_help':'Library / media root; Windows UNC paths are supported', 'convert_help':'Convert eligible reported files, keeping originals',
  'output_help':'Output root; relative directory structure is preserved', 'dry_help':'Read-only inspection and conversion plan; write nothing',
  'report_help':'Optional JSON report path (not written during DRY-RUN)', 'all_help':'Probe every non-known image (slow; non-media files may appear as inspection errors)',
  'images_help':'Allow unsupported images to single-frame PNG (animation, EXIF / GPS or HDR may be lost)',
  'timeout_help':'Per-file probe timeout in seconds; default 60', 'ct_help':'Per-file conversion timeout in seconds; default 7200',
  'tool_help':'Tool command or full executable path', 'lang_help':'Message language',
 }
}
LANG = 'zh-CN'
def tr(key, **values):
    return MESSAGES[LANG][key].format(**values)
def say(key, **values):
    print(tr(key, **values), flush=True)

def tool_path(value, name):
    result = shutil.which(value)
    if not result:
        raise ValueError(tr('tool_missing', tool=name))
    return result

def inside(path, parent):
    return path == parent or parent in path.parents

def discover(root, output, probe_all):
    """只遍历一次目录树；不跟随符号链接／Windows junction，排除 EG 缓存和输出。"""
    candidates, errors = [], []
    dirs = files = 0
    started = last = time.monotonic()
    def walk_error(error):
        row = {'path': str(error.filename), 'codes': ['walk_failed'], 'detail': str(error)}
        errors.append(row)
        say('issue', code='walk_failed', reason=tr('walk_failed') + ': ' + str(error), path=error.filename)
    for folder, children, names in os.walk(root, followlinks=False, onerror=walk_error):
        dirs += 1
        # isjunction 在 Python 3.12+ 提供；老版本借助 reparse 属性跳过目录重解析点。
        def allowed(name):
            path = Path(folder) / name
            if name.lower() in SKIP_DIRS or (output and inside(path.absolute(), output)):
                return False
            try:
                return not path.is_symlink() and not (getattr(path.lstat(), 'st_file_attributes', 0) & 0x400)
            except OSError as error:
                walk_error(error)
                return False
        children[:] = sorted(filter(allowed, children), key=str.casefold)
        for name in sorted(names, key=str.casefold):
            path = Path(folder) / name
            files += 1
            ext = path.suffix.lower()
            mime = mimetypes.guess_type(name)[0] or ''
            if not path.is_symlink() and (probe_all or ext in IMAGES | VIDEOS | EXTRA_IMAGES | EXTRA_VIDEOS or mime.startswith(('image/', 'video/', 'audio/'))):
                candidates.append(path)
            if time.monotonic() - last >= 1:
                say('walk', dirs=dirs, files=files, media=len(candidates), elapsed=time.monotonic()-started, path=path)
                last = time.monotonic()
        if time.monotonic() - last >= 1:
            say('walk', dirs=dirs, files=files, media=len(candidates), elapsed=time.monotonic()-started, path=folder)
            last = time.monotonic()
    say('walk', dirs=dirs, files=files, media=len(candidates), elapsed=time.monotonic()-started, path=root)
    return candidates, errors

def run_probe(executable, path, timeout):
    """不经 shell 执行，避免空格、中文、emoji 或特殊字符被 shell 解释。"""
    process = subprocess.Popen([executable, '-v', 'error', '-show_streams', '-show_format', '-of', 'json', str(path)],
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    start = time.monotonic()
    try:
        while True:
            try:
                stdout, stderr = process.communicate(timeout=min(2, timeout))
                break
            except subprocess.TimeoutExpired:
                if time.monotonic() - start >= timeout:
                    raise TimeoutError(tr('timeout', seconds=timeout))
                say('waiting', phase=tr('probe_phase'), elapsed=time.monotonic()-start, path=path)
        if process.returncode:
            raise ValueError(stderr.decode('utf-8', errors='replace')[-4000:])
        return json.loads(stdout)
    finally:
        if process.poll() is None:
            process.kill()
        process.communicate()

def inspect(path, probe, timeout):
    ext = path.suffix.lower()
    image = ext in IMAGES | EXTRA_IMAGES or (mimetypes.guess_type(path.name)[0] or '').startswith('image/')
    row = {'path': str(path), 'kind': 'image' if image else 'video', 'codes': [], 'conversion': 'none'}
    if ext not in IMAGES | VIDEOS:
        row['codes'].append('extension')
    if image:
        # HEIF 的映射属于 EG 既有逻辑，不把它错误标记为浏览器不兼容文件。
        if row['codes']:
            row['conversion'] = 'image'
        return row
    try:
        info = run_probe(probe, path, timeout)
        streams = info.get('streams', [])
        videos = [s for s in streams if s.get('codec_type') == 'video' and not s.get('disposition', {}).get('attached_pic')]
        audios = [s for s in streams if s.get('codec_type') == 'audio']
        # 默认视频流优先，与 ffmpeg 实际映射保持一致；音轨逐条检查和保留。
        videos.sort(key=lambda s: not s.get('disposition', {}).get('default', 0))
        row['streams'] = [{k: s.get(k) for k in ('index', 'codec_type', 'codec_name', 'profile', 'pix_fmt')} for s in streams]
        row['duration'] = info.get('format', {}).get('duration')
        if not videos:
            row['codes'].append('no_video')
            return row
        video = videos[0]
        row['video_index'] = video['index']
        webm = ext == '.webm'
        safe_video = video.get('codec_name') in ({'vp8', 'vp9', 'av1'} if webm else {'h264'}) and video.get('pix_fmt') in {'yuv420p', 'yuvj420p'}
        if not safe_video:
            row['codes'].append('video_codec')
        if any(a.get('codec_name') not in ({'opus', 'vorbis'} if webm else {'aac', 'mp3'}) for a in audios):
            row['codes'].append('audio_codec')
        if ext not in {'.mp4', '.m4v', '.webm'}:
            row['codes'].append('container')
        if row['codes']:
            row['conversion'] = 'video'
    except (OSError, ValueError, TimeoutError) as error:
        row['codes'].append('probe_failed')
        row['detail'] = str(error)
    return row

def target_path(row, root, output):
    source = Path(row['path'])
    # 保留原扩展名，避免 a.mov / a.avi 同时转换时都映射为 a.mp4。
    suffix = '.eg-compatible.mp4' if row['conversion'] == 'video' else '.eg-compatible.png'
    return output / source.relative_to(root).parent / (source.name + suffix)

def conversion_command(ffmpeg, row, temporary):
    command = [ffmpeg, '-hide_banner', '-nostdin', '-v', 'error', '-n', '-i', row['path']]
    if row['conversion'] == 'video':
        command += ['-map', f"0:{row['video_index']}", '-map', '0:a?', '-map_metadata', '0', '-map_chapters', '0',
                    '-c:v', 'libx264', '-preset', 'medium', '-crf', '20', '-pix_fmt', 'yuv420p',
                    '-vf', 'scale=ceil(iw/2)*2:ceil(ih/2)*2', '-c:a', 'aac', '-b:a', '192k', '-movflags', '+faststart']
    else:
        command += ['-map', '0:v:0', '-frames:v', '1', '-update', '1', '-c:v', 'png']
    return command + ['-progress', 'pipe:1', '-nostats', str(temporary)]

def convert(ffmpeg, row, target, index, total, timeout):
    """临时文件成功完成后才发布；失败／取消清理，已有目标绝不覆盖。"""
    target.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix='.eg-convert-', suffix=target.suffix, dir=target.parent)
    os.close(fd)
    temporary = Path(name)
    temporary.unlink()  # FFmpeg 使用 -n，因此临时目标应尚不存在。
    process = None
    try:
        with tempfile.TemporaryFile() as errors:
            process = subprocess.Popen(conversion_command(ffmpeg, row, temporary), stdout=subprocess.PIPE, stderr=errors)
            messages = queue.Queue()
            def reader():
                for line in process.stdout:
                    messages.put(line.decode('utf-8', errors='replace').strip())
                messages.put(None)
            thread = threading.Thread(target=reader, daemon=True)
            thread.start()
            started = last = time.monotonic()
            seconds, speed = 0.0, '?'
            while True:
                if time.monotonic() - started > timeout:
                    raise TimeoutError(tr('timeout', seconds=timeout))
                try:
                    line = messages.get(timeout=1)
                    if line is None:
                        break
                    key, _, value = line.partition('=')
                    if key == 'out_time_us':
                        try: seconds = max(0, int(value) / 1_000_000)
                        except ValueError: pass
                    elif key == 'speed': speed = value
                except queue.Empty:
                    pass
                if time.monotonic() - last >= 1:
                    try: duration = float(row.get('duration') or 0)
                    except (TypeError, ValueError): duration = 0
                    if not math.isfinite(duration): duration = 0
                    percent = f'{min(99.9, seconds/duration*100):.1f}%' if duration > 0 else '?%'
                    say('progress', index=index, total=total, percent=percent, seconds=seconds, speed=speed, path=row['path'])
                    last = time.monotonic()
            process.wait(timeout=max(1, timeout - (time.monotonic()-started)))
            thread.join(timeout=2)
            if process.returncode or not temporary.exists() or temporary.stat().st_size == 0:
                errors.seek(0, os.SEEK_END)
                errors.seek(max(0, errors.tell()-4000))
                raise ValueError(errors.read().decode('utf-8', errors='replace'))
        # Windows rename 不覆盖已存在目标；POSIX 使用硬链接实现同样的原子保护。
        if os.name == 'nt':
            os.rename(temporary, target)
        else:
            os.link(temporary, target)
    finally:
        if process and process.poll() is None:
            process.kill()
            process.wait()
        if process and process.stdout:
            process.stdout.close()
        temporary.unlink(missing_ok=True)

def main(argv=None):
    global LANG
    # 先读取语言，再建立完整参数解析器，让 --help 也遵循语言选择。
    pre = argparse.ArgumentParser(add_help=False)
    pre.add_argument('--lang', choices=MESSAGES, default='zh-CN')
    LANG = pre.parse_known_args(argv)[0].lang
    parser = argparse.ArgumentParser(description=tr('help'))
    parser.add_argument('root', type=Path, help=tr('root_help'))
    parser.add_argument('--lang', choices=MESSAGES, default=LANG, help=tr('lang_help'))
    for flag, key in [('convert','convert_help'), ('dry-run','dry_help'), ('probe-all','all_help'), ('convert-images','images_help')]:
        parser.add_argument('--'+flag, action='store_true', help=tr(key))
    parser.add_argument('--output', type=Path, help=tr('output_help'))
    parser.add_argument('--report', type=Path, help=tr('report_help'))
    for name in ['ffprobe', 'ffmpeg']:
        parser.add_argument('--'+name, default=name, help=tr('tool_help'))
    parser.add_argument('--probe-timeout', type=float, default=60, help=tr('timeout_help'))
    parser.add_argument('--convert-timeout', type=float, default=7200, help=tr('ct_help'))
    args = parser.parse_args(argv)
    if any(not math.isfinite(value) or value <= 0 for value in (args.probe_timeout, args.convert_timeout)):
        parser.error('--probe-timeout / --convert-timeout > 0')
    started = time.monotonic()
    root = args.root.resolve()
    output = args.output.resolve() if args.output else None
    if not root.is_dir():
        raise ValueError(tr('input_invalid', path=root))
    if args.convert and (not output or inside(root, output)):
        raise ValueError(tr('output_required'))
    probe = tool_path(args.ffprobe, 'ffprobe')
    ffmpeg = tool_path(args.ffmpeg, 'ffmpeg') if args.convert and not args.dry_run else args.ffmpeg
    if args.dry_run: say('dry')
    say('start', root=root)
    candidates, errors = discover(root, output, args.probe_all)
    issues = []
    for index, path in enumerate(candidates, 1):
        say('probe', index=index, total=len(candidates), issues=len(issues), path=path)
        row = inspect(path, probe, args.probe_timeout)
        if row['codes']:
            issues.append(row)
            for code in row['codes']:
                say('issue', code=code, reason=tr(code), path=path)
            if row.get('detail'): print(row['detail'], flush=True)
    eligible = [row for row in issues if row['conversion'] == 'video' or (row['conversion'] == 'image' and args.convert_images)]
    converted = failed = skipped = 0
    if args.convert:
        for index, row in enumerate(eligible, 1):
            target = target_path(row, root, output)
            row['target'] = str(target)
            if target.exists():
                skipped += 1
                row['result'] = 'existing_unverified'
                say('skip', path=target)
            elif args.dry_run:
                row['result'] = 'planned'
                say('plan', source=row['path'], target=target)
            else:
                try:
                    convert(ffmpeg, row, target, index, len(eligible), args.convert_timeout)
                    converted += 1
                    row['result'] = 'converted'
                    say('saved', path=target)
                except (OSError, ValueError, TimeoutError, subprocess.TimeoutExpired) as error:
                    failed += 1
                    row['result'] = 'failed'
                    row['conversion_error'] = str(error)
                    say('error', error=f"{row['path']}: {error}")
        for row in issues:
            if row['conversion'] == 'image' and not args.convert_images:
                say('issue', code='image_manual', reason=tr('image_manual'), path=row['path'])
    summary = dict(total=len(candidates), ok=len(candidates)-len(issues), issues=len(issues),
                   errors=len(errors)+sum('probe_failed' in row['codes'] for row in issues),
                   converted=converted, failed=failed, skipped=skipped, elapsed=time.monotonic()-started)
    if args.report and not args.dry_run:
        # 报告用独占创建，防止用户误把现有媒体路径作为报告文件覆盖。
        with args.report.open('x', encoding='utf-8') as handle:
            json.dump({'root': str(root), 'summary': summary, 'issues': issues, 'directory_errors': errors}, handle, ensure_ascii=False, indent=2)
        say('report', path=args.report)
    say('done', **summary)
    # 0=未发现问题；1=发现兼容性问题；2=访问／检查／转换错误；130=用户取消。
    return 2 if summary['errors'] or failed else (1 if issues else 0)

if __name__ == '__main__':
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, 'reconfigure'):
            stream.reconfigure(errors='replace')
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        say('cancel')
        sys.exit(130)
    except (OSError, ValueError) as error:
        say('error', error=error)
        sys.exit(2)
