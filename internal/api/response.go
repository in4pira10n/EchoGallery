package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Stable API text is keyed here instead of exposing implementation errors to
// the translation catalog. The Chinese text remains in the response for old
// clients and as a safe fallback when a locale is incomplete.
var stableAPIErrorKeys = map[string]string{
	"PWA 图标尚未上传":                "api.errors.pwaIconMissing",
	"media_id 不能为空":             "api.errors.mediaIDRequired",
	"media_ids 不能为空":            "api.errors.mediaIDsRequired",
	"poster 不存在":                "api.errors.posterNotFound",
	"上传文件不支持 seek":              "api.errors.uploadFileNotSeekable",
	"上传文件不能为空":                  "api.errors.uploadFileEmpty",
	"不支持的请求方法":                  "api.errors.methodNotAllowed",
	"仅 root 可执行此操作":             "api.errors.rootOnly",
	"仅可创建管理员或访客账号":              "api.errors.invalidRootRole",
	"仅支持 png/jpg/jpeg/gif/webp": "api.errors.imageFormatUnsupported",
	"仅支持上报视频播放错误":               "api.errors.playbackErrorReportUnsupported",
	"保存资源库选择失败":                 "api.errors.librarySelectionSaveFailed",
	"分享ID 无效":                   "api.errors.shareIDInvalid",
	"分享链接不存在或已过期":               "api.errors.shareNotFoundOrExpired",
	"刷新页面会话失败":                  "api.errors.pageSessionRefreshFailed",
	"加载用户配置失败":                  "api.errors.profileLoadFailed",
	"加载资源库服务失败":                 "api.errors.libraryServiceLoadFailed",
	"媒体 ID 无效":                  "api.errors.mediaIDInvalid",
	"媒体ID 无效":                   "api.errors.mediaIDInvalid",
	"媒体上传服务未配置":                 "api.errors.mediaUploadUnavailable",
	"媒体不存在":                     "api.errors.mediaNotFound",
	"媒体服务未配置":                   "api.errors.mediaServiceUnavailable",
	"媒体编号无效":                    "api.errors.mediaIDInvalid",
	"媒体重命名服务未配置":                "api.errors.mediaRenameUnavailable",
	"当前不是初始化模式":                 "api.errors.notInitializationMode",
	"当前不是资源库恢复模式":               "api.errors.notLibraryRecoveryMode",
	"当前仅支持常见媒体格式上传（mp4/m4v/mov/webm/mkv/avi/wmv/wma/mpeg/ts/3gp/ogv）": "api.errors.uploadFormatUnsupported",
	"当前分享不支持下载":           "api.errors.shareDownloadUnsupported",
	"当前分享不支持该媒体访问":        "api.errors.shareMediaUnsupported",
	"当前分享不是相册类型":          "api.errors.shareNotAlbum",
	"当前媒体服务不支持智能相册":       "api.errors.smartAlbumUnavailable",
	"当前实例不支持停止资源库构建":      "api.errors.libraryBuildStopUnavailable",
	"当前实例不支持全局退出":         "api.errors.shutdownUnavailable",
	"当前实例不支持刷新视频缩略图":      "api.errors.videoThumbnailRefreshUnavailable",
	"当前实例不支持取消媒体元数据修正":    "api.errors.exifBackfillCancelUnavailable",
	"当前实例不支持取消批量扫描":       "api.errors.batchScanCancelUnavailable",
	"当前实例不支持取消批量缩略图任务":    "api.errors.batchThumbnailCancelUnavailable",
	"当前实例不支持取消播放缓存构建":     "api.errors.playbackCacheCancelUnavailable",
	"当前实例不支持取消视频缩略图刷新":    "api.errors.videoThumbnailRefreshCancelUnavailable",
	"当前实例不支持媒体元数据修正":      "api.errors.exifBackfillUnavailable",
	"当前实例不支持完成后退出":        "api.errors.libraryBuildExitUnavailable",
	"当前实例不支持批量扫描勾选":       "api.errors.batchScanSelectionUnavailable",
	"当前实例不支持批量扫描完成后退出":    "api.errors.batchScanExitUnavailable",
	"当前实例不支持批量扫描资源库":      "api.errors.batchScanUnavailable",
	"当前实例不支持批量构建全部资源库缩略图": "api.errors.batchThumbnailBuildUnavailable",
	"当前实例不支持批量生成缩略图":      "api.errors.thumbnailBuildUnavailable",
	"当前实例不支持批量缩略图勾选":      "api.errors.batchThumbnailSelectionUnavailable",
	"当前实例不支持批量缩略图完成后退出":   "api.errors.batchThumbnailExitUnavailable",
	"当前实例不支持播放缓存构建":       "api.errors.playbackCacheBuildUnavailable",
	"当前实例不支持播放缓存管理":       "api.errors.playbackCacheUnavailable",
	"当前实例不支持网页更新重启":       "api.errors.webUpdateRestartUnavailable",
	"当前实例不支持网页重启":         "api.errors.webRestartUnavailable",
	"当前实例不支持资源库检查":        "api.errors.libraryScanUnavailable",
	"当前尚未完成初始化，无法切换用户":    "api.errors.setupRequired",
	"当前浏览器暂不支持直接播放该文件，请先在“更多操作”里转换为受支持的格式": "api.errors.browserPlaybackUnsupported",
	"当前资源库正被其他用户占用": "api.errors.libraryOccupied",
	"打包下载失败":        "api.errors.downloadFailed",
	"打开上传文件失败":      "api.errors.uploadOpenFailed",
	"批量工作流的迁移、旧文件清理和播放缓存阶段已暂停": "api.errors.batchLegacyStagesPaused",
	"接口不存在":                         "api.errors.routeNotFound",
	"播放兼容缓存服务未配置":                   "api.errors.playbackCacheServiceUnavailable",
	"播放兼容转换服务未配置":                   "api.errors.playbackConversionUnavailable",
	"播放错误报告格式无效":                    "api.errors.playbackErrorReportInvalid",
	"文件夹回收站批次无效":                    "api.errors.folderTrashBatchInvalid",
	"文件夹路径不能为空":                     "api.errors.folderPathRequired",
	"无效的用户角色":                       "api.errors.invalidUserRole",
	"无效的请求体":                        "api.errors.invalidRequestBody",
	"无效的资源库请求":                      "api.errors.invalidLibraryRequest",
	"无法写入播放错误日志":                    "api.errors.playbackErrorLogWriteFailed",
	"最多选择 32 个相册":                   "api.errors.albumLimitExceeded",
	"未登录":                           "api.errors.notLoggedIn",
	"未登录或登录已过期":                     "api.errors.authExpired",
	"检查资源库占用失败":                     "api.errors.libraryOccupancyCheckFailed",
	"浏览器设置过大":                       "api.errors.browserSettingsTooLarge",
	"照片/视频ID 无效":                    "api.errors.mediaIDInvalid",
	"照片/视频不存在":                      "api.errors.mediaNotFound",
	"生成令牌失败":                        "api.errors.tokenGenerationFailed",
	"用户不存在":                         "api.errors.userNotFound",
	"用户名或密码错误":                      "api.errors.invalidCredentials",
	"用户已失效":                         "api.errors.userExpired",
	"相册 ID 无效":                      "api.errors.albumIDInvalid",
	"相册ID 无效":                       "api.errors.albumIDInvalid",
	"相册不存在":                         "api.errors.albumNotFound",
	"缩略图不存在":                        "api.errors.thumbnailNotFound",
	"缩略图预热不可用":                      "api.errors.thumbnailWarmUnavailable",
	"缺少上传文件":                        "api.errors.uploadFileRequired",
	"缺少媒体文件字段":                      "api.errors.mediaFileFieldRequired",
	"视频播放偏好服务未配置":                   "api.errors.videoPlaybackPreferenceUnavailable",
	"访客用户无权执行此操作":                   "api.errors.guestForbidden",
	"请填写资源库路径":                      "api.errors.libraryPathRequired",
	"请求参数无效":                        "api.errors.invalidParameters",
	"请求格式错误":                        "api.errors.invalidRequestFormat",
	"请选择 64 MB 以内的 EchoGallery 配置包": "api.errors.transferFileInvalid",
	"请选择配置包":                        "api.errors.transferFileRequired",
	"读取上传文件失败":                      "api.errors.uploadReadFailed",
	"读取调试媒体失败":                      "api.errors.debugMediaReadFailed",
	"读取配置包失败":                       "api.errors.transferReadFailed",
	"调试媒体不存在":                       "api.errors.debugMediaNotFound",
	"调试媒体文件不存在":                     "api.errors.debugMediaFileNotFound",
	"调试媒体路径无效":                      "api.errors.debugMediaPathInvalid",
	"资源不存在":                         "api.errors.resourceNotFound",
	"资源库占用状态检查失败":                   "api.errors.libraryOccupancyStatusFailed",
	"资源库同步失败，暂未释放占用，请重试":            "api.errors.librarySyncFailed",
	"资源库同步失败，未切换":                   "api.errors.librarySwitchSyncFailed",
	"旧资源库同步失败，暂未释放占用，请重试":           "api.errors.previousLibrarySyncFailed",
	"资源库正由其他 Gallery 使用":            "api.errors.portableLibraryOccupied",
	"资源库头像生成失败":                     "api.errors.libraryLogoGenerateFailed",
	"资源库路径不能为空":                     "api.errors.libraryPathRequired",
	"输入路径不是文件夹":                     "api.errors.pathNotDirectory",
	"配置包大小必须在 64 MB 以内":             "api.errors.transferSizeTooLarge",
}

var stableAPIMessageKeys = map[string]string{
	"登录成功": "pages.login.messages.loginSuccess",
	"访客账号创建成功，请登录后继续":  "api.messages.visitorCreated",
	"管理员账号创建成功，请登录后继续": "api.messages.adminCreated",
	"用户已创建":              "api.messages.userCreated",
	"用户已更新":              "api.messages.userUpdated",
	"用户已删除":              "api.messages.userDeleted",
	"已转换为受支持的格式":         "api.messages.playbackConversionCompleted",
	"已从相册移除":             "api.messages.removedFromAlbum",
	"已使用系统播放器打开":         "api.messages.openedInSystemPlayer",
	"已在文件管理器中定位":         "api.messages.revealedInFileManager",
	"已永久删除":              "api.messages.permanentlyDeleted",
	"已添加到相册":             "api.messages.addedToAlbum",
	"已移入回收站":             "api.messages.movedToTrash",
	"已退出登录":              "api.messages.loggedOut",
	"已取消收藏":              "api.messages.removedFromFavorites",
	"已加入特别喜欢":            "api.messages.addedToSpecialFavorites",
	"已加入个人收藏":            "api.messages.addedToFavorites",
	"恢复成功":               "api.messages.restored",
	"文件夹及其内容已恢复":         "api.messages.folderRestored",
	"文件夹及其内容已永久删除":       "api.messages.folderPermanentlyDeleted",
	"分享链接已删除":            "api.messages.shareDeleted",
	"初始化完成，服务正在重启":       "api.messages.initializationCompleted",
	"回收站已清空":             "api.messages.trashEmptied",
	"媒体已重命名":             "api.messages.mediaRenamed",
	"相册已删除":              "api.messages.albumDeleted",
	"资源库已切换，服务正在重启":      "api.messages.librarySwitched",
	"设置已保存":              "api.messages.settingsSaved",
	"服务正在重启":             "api.messages.serviceRestarting",
	"正在退出 EchoGallery":   "api.messages.appShuttingDown",
	"资源库头像已上传":           "api.messages.libraryLogoUploaded",
	"资源库图像已移除":           "api.messages.libraryLogoRemoved",
	"PWA 图标已上传":          "api.messages.pwaIconUploaded",
	"视频上传成功":             "api.messages.videoUploaded",
	"视频缩略图刷新完成":          "api.messages.videoThumbnailsRefreshed",
	"配置已导入，应用正在重启":       "api.messages.transferImportedRestarting",
	"配置已导入，应用需要重启":       "api.messages.transferImportedRequiresRestart",
	"已切换到该用户，请继续完成初始化。":  "api.messages.userSwitchedForSetup",
	"登录成功，正在切换到该用户的资源库。": "api.messages.loginSwitchingLibrary",
	"没有可更新的资源库头像":        "api.messages.noLibraryLogosToRefresh",
	"当前没有扫描任务":           "api.status.noScanTask",
	"当前没有批量扫描任务":         "api.status.noBatchScanTask",
	"当前没有批量缩略图任务":        "api.status.noBatchThumbnailTask",
	"当前没有播放兼容缓存任务":       "api.status.noPlaybackCacheTask",
	"当前没有缩略图任务":          "api.status.noThumbnailTask",
	"当前没有视频缩略图任务":        "api.status.noVideoThumbnailTask",
	"当前没有 EXIF 修正任务":     "api.status.noEXIFBackfillTask",
	"正在扫描媒体并生成标准缩略图…":    "api.status.thumbnailBuildPreparing",
	"正在排队标准缩略图任务…":       "api.status.thumbnailBuildQueued",
	"标准缩略图已生成完成":         "api.status.thumbnailBuildCompleted",
	"已取消标准缩略图生成":         "api.status.thumbnailBuildCancelled",
	"标准缩略图生成失败":          "api.status.thumbnailBuildFailed",
	"正在取消缩略图生成…":         "api.status.thumbnailBuildCancelling",
	"正在准备修正 EXIF 信息…":    "api.status.exifBackfillPreparing",
	"正在扫描媒体…":            "api.status.exifBackfillScanning",
	"正在取消 EXIF 信息修正…":    "api.status.exifBackfillCancelling",
	"已取消 EXIF 信息修正":      "api.status.exifBackfillCancelled",
	"EXIF 信息修正失败":        "api.status.exifBackfillFailed",
	"EXIF 信息修正完成":        "api.status.exifBackfillCompleted",
	"正在扫描视频并刷新缩略图…":      "api.status.videoThumbnailRefreshPreparing",
	"正在取消视频缩略图刷新…":       "api.status.videoThumbnailRefreshCancelling",
	"已取消视频缩略图刷新":         "api.status.videoThumbnailRefreshCancelled",
	"视频缩略图刷新失败":          "api.status.videoThumbnailRefreshFailed",
	"没有需要刷新的视频":          "api.status.videoThumbnailRefreshNoWork",
	"正在准备播放兼容缓存…":        "api.status.playbackCachePreparing",
	"正在取消播放兼容缓存任务…":      "api.status.playbackCacheCancelling",
	"播放兼容缓存构建失败":         "api.status.playbackCacheFailed",
	"已取消播放兼容缓存任务":        "api.status.playbackCacheCancelled",
	"跳过浏览器已支持的视频":        "api.status.playbackCacheSkippedSupported",
	"播放兼容缓存构建完成":         "api.status.playbackCacheCompleted",
}

func dynamicAPIMessage(value string) (string, map[string]any) {
	if key := stableAPIMessageKeys[value]; key != "" {
		return key, nil
	}
	if prefix, rest, ok := strings.Cut(value, "… 已完成 "); ok {
		countText := strings.TrimSuffix(rest, " 项")
		count, err := strconv.Atoi(countText)
		if err == nil {
			switch {
			case strings.HasPrefix(prefix, "正在生成"):
				kind := strings.TrimPrefix(prefix, "正在生成")
				if key := thumbnailProgressKey("generating", kind); key != "" {
					return key, map[string]any{"count": count}
				}
			case strings.HasPrefix(prefix, "正在检查"):
				kind := strings.TrimPrefix(prefix, "正在检查")
				if key := thumbnailProgressKey("checking", kind); key != "" {
					return key, map[string]any{"count": count}
				}
			case strings.HasSuffix(prefix, "生成失败"):
				kind := strings.TrimSuffix(prefix, "生成失败")
				if key := thumbnailProgressKey("failed", kind); key != "" {
					return key, map[string]any{"count": count}
				}
			case prefix == "正在刷新视频缩略图":
				return "api.status.videoThumbnailRefreshProgress", map[string]any{"count": count}
			case prefix == "正在修正 EXIF 信息":
				return "api.status.exifBackfillProgress", map[string]any{"count": count}
			}
		}
	}
	if strings.HasPrefix(value, "正在准备 ") && strings.HasSuffix(value, " 的播放缓存") {
		name := strings.TrimSuffix(strings.TrimPrefix(value, "正在准备 "), " 的播放缓存")
		return "api.status.playbackCachePreparingMedia", map[string]any{"name": name}
	}
	if strings.HasSuffix(value, " 播放缓存构建失败") {
		name := strings.TrimSuffix(value, " 播放缓存构建失败")
		return "api.status.playbackCacheMediaFailed", map[string]any{"name": name}
	}
	if strings.HasSuffix(value, " 播放缓存已就绪") {
		name := strings.TrimSuffix(value, " 播放缓存已就绪")
		return "api.status.playbackCacheMediaReady", map[string]any{"name": name}
	}
	return "", nil
}

func thumbnailProgressKey(action, kind string) string {
	kindKeys := map[string]string{
		"标准缩略图": "standard",
		"大图":    "large",
		"列表图":   "list",
		"缩略图":   "generic",
	}
	kindKey := kindKeys[kind]
	if kindKey == "" {
		return ""
	}
	return "api.status.thumbnail" + strings.ToUpper(action[:1]) + action[1:] + strings.ToUpper(kindKey[:1]) + kindKey[1:]
}

func apiKeyForField(field, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if field == "error" {
		return stableAPIErrorKeys[value]
	}
	return stableAPIMessageKeys[value]
}

// apiResponseKeyMiddleware adds keys to existing JSON responses without
// changing the response shape used by older clients. Binary API responses are
// passed through immediately, so downloads and media files are never buffered.
func apiResponseKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c == nil || c.Request == nil || !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Next()
			return
		}

		original := c.Writer
		writer := &apiResponseWriter{ResponseWriter: original, status: http.StatusOK}
		c.Writer = writer
		defer func() {
			c.Writer = original
			if recovered := recover(); recovered != nil {
				panic(recovered)
			}
			finishAPIResponse(original, writer)
		}()
		c.Next()
	}
}

func finishAPIResponse(original gin.ResponseWriter, writer *apiResponseWriter) {
	if !writer.capture {
		if (writer.headerWritten || writer.status != http.StatusOK) && !writer.forwarded {
			original.WriteHeader(writer.status)
		}
		return
	}
	body := append([]byte(nil), writer.body.Bytes()...)
	if len(body) == 0 {
		if writer.headerWritten {
			original.WriteHeader(writer.status)
		}
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if value, ok := payload["error"].(string); ok {
			if _, exists := payload["error_key"]; !exists {
				if key := apiKeyForField("error", value); key != "" {
					payload["error_key"] = key
				}
			}
		}
		if value, ok := payload["message"].(string); ok {
			if _, exists := payload["message_key"]; !exists {
				if key, params := dynamicAPIMessage(value); key != "" {
					payload["message_key"] = key
					if len(params) > 0 {
						payload["message_params"] = params
					}
				}
			}
		}
		if updated, err := json.Marshal(payload); err == nil {
			body = updated
		}
	}
	original.WriteHeader(writer.status)
	_, _ = original.Write(body)
}

type apiResponseWriter struct {
	gin.ResponseWriter
	body          bytes.Buffer
	status        int
	size          int
	headerWritten bool
	capture       bool
	forwarded     bool
}

const maxKeyedAPIResponseBytes = 1 << 20

func (w *apiResponseWriter) WriteHeader(code int) {
	if code > 0 && !w.headerWritten {
		w.status = code
	}
}

func (w *apiResponseWriter) WriteHeaderNow() {
	if !w.headerWritten {
		w.headerWritten = true
		w.size = 0
	}
}

func (w *apiResponseWriter) shouldCapture() bool {
	contentType := strings.ToLower(w.Header().Get("Content-Type"))
	return contentType == "" || strings.Contains(contentType, "application/json") || strings.Contains(contentType, "text/json")
}

func (w *apiResponseWriter) forwardHeader() {
	if w.forwarded {
		return
	}
	w.forwarded = true
	w.ResponseWriter.WriteHeader(w.status)
}

func (w *apiResponseWriter) Write(data []byte) (int, error) {
	if w.shouldCapture() {
		if w.body.Len()+len(data) > maxKeyedAPIResponseBytes {
			w.capture = false
			w.WriteHeaderNow()
			w.forwardHeader()
			if w.body.Len() > 0 {
				if _, err := w.ResponseWriter.Write(w.body.Bytes()); err != nil {
					return 0, err
				}
				w.size += w.body.Len()
				w.body.Reset()
			}
			n, err := w.ResponseWriter.Write(data)
			w.size += n
			return n, err
		}
		w.capture = true
		w.WriteHeaderNow()
		n, err := w.body.Write(data)
		w.size += n
		return n, err
	}
	w.WriteHeaderNow()
	w.forwardHeader()
	n, err := w.ResponseWriter.Write(data)
	w.size += n
	return n, err
}

func (w *apiResponseWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

func (w *apiResponseWriter) Status() int { return w.status }

func (w *apiResponseWriter) Size() int { return w.size }

func (w *apiResponseWriter) Written() bool { return w.headerWritten }

func (w *apiResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.WriteHeaderNow()
	w.forwardHeader()
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *apiResponseWriter) CloseNotify() <-chan bool {
	if notifier, ok := w.ResponseWriter.(http.CloseNotifier); ok {
		return notifier.CloseNotify()
	}
	return make(chan bool)
}

func (w *apiResponseWriter) Flush() {
	w.WriteHeaderNow()
	w.forwardHeader()
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *apiResponseWriter) Pusher() http.Pusher {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher
	}
	return nil
}

var _ gin.ResponseWriter = (*apiResponseWriter)(nil)
