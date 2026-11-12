"""使用临时生成的媒体验证检查／转换；不访问用户资源库。"""
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / 'scripts' / 'check_media.py'
spec = importlib.util.spec_from_file_location('check_media', SCRIPT)
media = importlib.util.module_from_spec(spec)
spec.loader.exec_module(media)


class MediaAuditTests(unittest.TestCase):
    def test_translations_and_safe_destinations(self):
        self.assertEqual(set(media.MESSAGES['zh-CN']), set(media.MESSAGES['en-US']))
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            row = {'path': str(root / '🐱' / 'a.mov'), 'conversion': 'video'}
            self.assertEqual(media.target_path(row, root, root / 'output'), root / 'output' / '🐱' / 'a.mov.eg-compatible.mp4')
            row['path'] = str(root / '🐱' / 'a.avi')
            self.assertNotEqual(media.target_path(row, root, root / 'output').name, 'a.mov.eg-compatible.mp4')

    def test_discovery_excludes_cache_output_and_symbolic_loops(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            for name in ['nested/صورة.mp4', '.echogallery/cache.mp4', 'out/converted.mp4', 'notes.txt']:
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text('fixture')
            try:
                (root / 'nested' / 'loop').symlink_to(root, target_is_directory=True)
            except OSError:
                pass  # Windows 非管理员环境可能没有符号链接权限。
            paths, errors = media.discover(root, root / 'out', False)
            self.assertEqual(paths, [root / 'nested' / 'صورة.mp4'])
            self.assertEqual(errors, [])

    def test_probe_errors_are_reported_without_attempting_conversion(self):
        with patch.object(media, 'run_probe', side_effect=TimeoutError('timeout')):
            row = media.inspect(Path('/sample.mp4'), 'ffprobe', 1)
        self.assertIn('probe_failed', row['codes'])
        self.assertEqual(row['conversion'], 'none')

    @unittest.skipUnless(shutil.which('ffmpeg') and shutil.which('ffprobe'), 'FFmpeg tools required')
    def test_dry_run_conversion_roundtrip_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder) / 'input'
            root.mkdir()
            source = root / '子目录 русский العربية 🐱' / 'clip.mp4'
            source.parent.mkdir()
            subprocess.run(['ffmpeg', '-v', 'error', '-f', 'lavfi', '-i', 'color=size=64x64:rate=10',
                            '-f', 'lavfi', '-i', 'sine=frequency=440', '-map', '0:v', '-map', '1:a', '-map', '1:a',
                            '-t', '0.4', '-c:v', 'mpeg4', '-c:a:0', 'aac', '-c:a:1', 'ac3', str(source)], check=True)
            digest = hashlib.sha256(source.read_bytes()).hexdigest()
            row = media.inspect(source, shutil.which('ffprobe'), 10)
            self.assertIn('video_codec', row['codes'])
            self.assertIn('audio_codec', row['codes'])
            output, report = Path(folder) / 'output', Path(folder) / 'report.json'
            self.assertEqual(media.main([str(root), '--convert', '--output', str(output), '--dry-run', '--report', str(report)]), 1)
            self.assertFalse(output.exists())
            self.assertFalse(report.exists())
            self.assertEqual(media.main([str(root), '--convert', '--output', str(output), '--report', str(report)]), 1)
            target = media.target_path(row, root, output)
            converted = media.inspect(target, shutil.which('ffprobe'), 10)
            self.assertEqual(converted['codes'], [])
            self.assertEqual(sum(s['codec_type'] == 'audio' for s in converted['streams']), 2)
            self.assertEqual(hashlib.sha256(source.read_bytes()).hexdigest(), digest)
            self.assertEqual(json.loads(report.read_text())['summary']['converted'], 1)
            converted_digest = hashlib.sha256(target.read_bytes()).hexdigest()
            self.assertEqual(media.main([str(root), '--convert', '--output', str(output)]), 1)
            self.assertEqual(hashlib.sha256(target.read_bytes()).hexdigest(), converted_digest)
            self.assertEqual(list(output.rglob('.eg-convert-*')), [])
            broken = dict(row, video_index=999)
            failed_target = output / 'failed.mp4'
            with self.assertRaises(ValueError):
                media.convert(shutil.which('ffmpeg'), broken, failed_target, 1, 1, 10)
            self.assertFalse(failed_target.exists())
            self.assertEqual(list(output.rglob('.eg-convert-*')), [])
            self.assertEqual(hashlib.sha256(source.read_bytes()).hexdigest(), digest)


if __name__ == '__main__':
    unittest.main()
