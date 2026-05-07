package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

type SetupMode string

const (
	SetupModeInit            SetupMode = "init"
	SetupModeLibraryRecovery SetupMode = "library_recovery"
)

type SetupState struct {
	Mode               SetupMode        `json:"mode"`
	Message            string           `json:"message"`
	Libraries          []config.Library `json:"libraries"`
	CurrentStoragePath string           `json:"current_storage_path"`
	Port               int              `json:"port"`
}

type setupController struct {
	state   SetupState
	restart func() error
}

type setupInitRequest struct {
	Port          int              `json:"port"`
	Libraries     []config.Library `json:"libraries"`
	StoragePath   string           `json:"storage_path"`
	ThumbnailDir  string           `json:"thumbnail_dir"`
	ThumbnailSize int              `json:"thumbnail_size"`
	TrashDir      string           `json:"trash_dir"`
	Username      string           `json:"username"`
	Password      string           `json:"password"`
}

type setupSelectLibraryRequest struct {
	Name        string `json:"name"`
	StoragePath string `json:"storage_path"`
}

func NewSetupRouterWithStatic(staticFS fs.FS, state SetupState, restart func() error) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.GET("/static/*filepath", gin.WrapH(buildStaticHandler(staticFS)))

	ctrl := &setupController{state: state, restart: restart}
	r.GET("/", ctrl.handleSetupPage())
	r.GET("/api/setup/state", ctrl.handleGetSetupState())
	r.POST("/api/setup/init", ctrl.handleInitialize())
	r.POST("/api/setup/select-library", ctrl.handleSelectLibrary())

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		c.Redirect(http.StatusSeeOther, "/")
	})
	return r
}

func (s *setupController) handleGetSetupState() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, s.state)
	}
}

func (s *setupController) handleInitialize() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.state.Mode != SetupModeInit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前不是初始化模式"})
			return
		}
		var req setupInitRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
			return
		}
		cfg, err := config.CreateInitialConfig(config.InitialConfigParams{
			Port:          req.Port,
			Libraries:     req.Libraries,
			StoragePath:   req.StoragePath,
			ThumbnailDir:  req.ThumbnailDir,
			ThumbnailSize: req.ThumbnailSize,
			TrashDir:      req.TrashDir,
			Username:      req.Username,
			Password:      req.Password,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		for _, library := range cfg.Libraries {
			if err := os.MkdirAll(library.Path, 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建资源库目录失败: %v", err)})
				return
			}
		}
		if thumbDir, err := cfg.ThumbnailStoragePath(); err == nil {
			if err := os.MkdirAll(thumbDir, 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建缩略图目录失败: %v", err)})
				return
			}
		}
		if trashDir, err := cfg.TrashPath(); err == nil {
			if err := os.MkdirAll(trashDir, 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建回收站目录失败: %v", err)})
				return
			}
		}
		if err := cfg.Save(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if s.restart != nil {
			if err := s.restart(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "初始化完成，服务正在重启"})
	}
}

func (s *setupController) handleSelectLibrary() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.state.Mode != SetupModeLibraryRecovery {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前不是资源库恢复模式"})
			return
		}
		var req setupSelectLibraryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
			return
		}
		cfg, err := config.Load()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		selected := strings.TrimSpace(req.StoragePath)
		if selected == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "资源库路径不能为空"})
			return
		}
		found := false
		for _, library := range cfg.Libraries {
			if config.NormalizeStoragePath(library.Path) == config.NormalizeStoragePath(selected) {
				cfg.StoragePath = library.Path
				found = true
				break
			}
		}
		if !found {
			name := strings.TrimSpace(req.Name)
			if name == "" {
				name = fmt.Sprintf("资源库 %d", len(cfg.Libraries)+1)
			}
			if err := os.MkdirAll(selected, 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建资源库目录失败: %v", err)})
				return
			}
			cfg.Libraries = append(cfg.Libraries, config.Library{Name: name, Path: selected})
			cfg.StoragePath = selected
		}
		if err := cfg.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if s.restart != nil {
			if err := s.restart(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "资源库已切换，服务正在重启"})
	}
}

func (s *setupController) handleSetupPage() gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, _ := json.Marshal(s.state)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(buildSetupPageHTML(string(payload))))
	}
}

func buildSetupPageHTML(payload string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN" data-theme="">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>EchoGallery 设置</title>
  <link rel="stylesheet" href="/static/app.css">
</head>
<body>
<script>
  (function(){
    var t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    document.documentElement.dataset.theme = t;
    window.__SETUP__ = %s;
  })();
</script>
<div class="login-wrap">
  <div class="login-card setup-card" id="setup-root"></div>
</div>
<script>
(function () {
  var state = window.__SETUP__ || {};
  var root = document.getElementById('setup-root');

  function esc(text) {
    return String(text || '').replace(/[&<>"]/g, function (m) {
      return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m];
    });
  }

  function renderInit() {
    root.className = 'login-card setup-card';
    var libraries = (state.libraries || []).length ? state.libraries : [{name:'默认资源库', path:''}];
    root.innerHTML = '<div class="login-logo">EchoGallery</div>' +
      '<div class="setup-title">网页初始化</div>' +
      '<p class="setup-desc">首次启动不再使用命令行向导，请在这里完成端口、资源库和管理员账号配置。</p>' +
      '<div class="form-group"><label class="form-label" for="setup-port">服务端口</label><input class="input" id="setup-port" type="number" min="1" max="65535" value="' + (state.port || 8080) + '"></div>' +
      '<div class="form-group"><label class="form-label" for="setup-thumbnail-dir">缩略图目录（可选）</label><input class="input" id="setup-thumbnail-dir" type="text" placeholder="默认使用应用数据目录下的 thumbnails"></div>' +
      '<div class="form-group"><label class="form-label" for="setup-trash-dir">回收站目录（可选）</label><input class="input" id="setup-trash-dir" type="text" placeholder="默认使用应用数据目录下的 Trash"></div>' +
      '<div class="form-group"><label class="form-label">资源库</label><div id="setup-libraries"></div><div class="settings-actions"><button class="btn" id="setup-add-library" type="button">添加资源库</button></div></div>' +
      '<div class="form-group"><label class="form-label" for="setup-username">管理员用户名</label><input class="input" id="setup-username" type="text" value="admin"></div>' +
      '<div class="form-group"><label class="form-label" for="setup-password">管理员密码</label><input class="input" id="setup-password" type="password" placeholder="至少 6 位"></div>' +
      '<button class="btn btn-primary" style="width:100%%;justify-content:center" id="setup-submit">保存并重启</button>' +
      '<div class="login-error" id="setup-error"></div>';
    var list = document.getElementById('setup-libraries');

    function renderLibraries(items) {
      list.innerHTML = items.map(function (library, index) {
        return '<div class="settings-library-row setup-library-row">' +
          '<input class="input setup-library-name" type="text" maxlength="32" placeholder="资源库名称" value="' + esc(library.name || ('资源库 ' + (index + 1))) + '">' +
          '<input class="input setup-library-path" type="text" placeholder="资源库路径" value="' + esc(library.path || '') + '">' +
          '<button class="btn btn-danger btn-sm setup-library-remove" type="button">删除</button>' +
        '</div>';
      }).join('');
      Array.prototype.forEach.call(list.querySelectorAll('.setup-library-remove'), function (btn) {
        btn.addEventListener('click', function () {
          if (list.children.length <= 1) return;
          btn.parentNode.remove();
        });
      });
    }

    renderLibraries(libraries);
    document.getElementById('setup-add-library').addEventListener('click', function () {
      libraries.push({name:'资源库 ' + (list.children.length + 1), path:''});
      renderLibraries(Array.prototype.map.call(list.children, function (row, index) {
        return {
          name: row.querySelector('.setup-library-name').value || ('资源库 ' + (index + 1)),
          path: row.querySelector('.setup-library-path').value || ''
        };
      }).concat([{name:'资源库 ' + (list.children.length + 1), path:''}]));
    });

    document.getElementById('setup-submit').addEventListener('click', async function () {
      var errorEl = document.getElementById('setup-error');
      errorEl.textContent = '';
      var rows = Array.prototype.map.call(list.querySelectorAll('.setup-library-row'), function (row, index) {
        return {
          name: row.querySelector('.setup-library-name').value.trim() || ('资源库 ' + (index + 1)),
          path: row.querySelector('.setup-library-path').value.trim()
        };
      }).filter(function (item) { return item.path; });
      if (!rows.length) {
        errorEl.textContent = '请至少填写一个资源库路径';
        return;
      }
      var btn = this;
      btn.disabled = true;
      btn.textContent = '保存中…';
      try {
        var resp = await fetch('/api/setup/init', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({
            port: parseInt(document.getElementById('setup-port').value, 10) || 8080,
            thumbnail_dir: document.getElementById('setup-thumbnail-dir').value.trim(),
            trash_dir: document.getElementById('setup-trash-dir').value.trim(),
            libraries: rows,
            storage_path: rows[0].path,
            username: document.getElementById('setup-username').value.trim(),
            password: document.getElementById('setup-password').value
          })
        });
        var data = await resp.json();
        if (!resp.ok) throw new Error(data.error || '初始化失败');
        btn.textContent = '正在重启…';
        errorEl.style.color = 'var(--accent)';
        errorEl.textContent = data.message || '初始化完成，服务正在重启';
        setTimeout(function () { location.reload(); }, 2000);
      } catch (e) {
        btn.disabled = false;
        btn.textContent = '保存并重启';
        errorEl.style.color = 'var(--danger)';
        errorEl.textContent = e.message || '初始化失败';
      }
    });
  }

  function renderRecovery() {
    root.className = 'login-card setup-card setup-recovery-card';
    var libraries = state.libraries || [];
    root.innerHTML =
      '<div class="setup-recovery-hero">' +
        '<div class="setup-recovery-brand"><span class="setup-recovery-mark">E</span><span>EchoGallery</span></div>' +
        '<span class="setup-recovery-status">需要选择资源库</span>' +
        '<h1>资源库恢复</h1>' +
        '<p>' + esc(state.message || '当前资源库不可用，请选择其他资源库后重启。') + '</p>' +
        '<div class="setup-recovery-current"><span>当前路径</span><strong>' + esc(state.current_storage_path || '未记录') + '</strong></div>' +
      '</div>' +
      '<div class="setup-recovery-grid">' +
        '<section class="setup-recovery-section">' +
          '<div class="setup-recovery-section-head"><strong>选择已有资源库</strong><span>' + libraries.length + ' 个记录</span></div>' +
          '<div class="setup-library-chooser" id="setup-library-chooser">' + (libraries.length ? libraries.map(function (library, index) {
            var currentClass = library.path === state.current_storage_path ? ' current' : '';
            var current = library.path === state.current_storage_path ? '<em>当前不可用</em>' : '';
            return '<button class="setup-library-option' + currentClass + '" type="button" data-path="' + esc(library.path) + '">' +
              '<span class="setup-library-option-dot">' + (index + 1) + '</span>' +
              '<span class="setup-library-option-main">' +
                '<strong>' + esc(library.name || library.path) + '</strong>' +
                '<small>' + esc(library.path) + '</small>' +
              '</span>' +
              current +
            '</button>';
          }).join('') : '<div class="setup-recovery-empty">config.json 中还没有可切换的资源库记录。</div>') + '</div>' +
        '</section>' +
        '<section class="setup-recovery-section setup-recovery-create">' +
          '<div class="setup-recovery-section-head"><strong>新建资源库</strong><span>自动创建目录</span></div>' +
          '<div class="form-group"><label class="form-label" for="setup-new-library-name">资源库名称</label><input class="input" id="setup-new-library-name" type="text" placeholder="例如：新资源库"></div>' +
          '<div class="form-group"><label class="form-label" for="setup-new-library-path">资源库路径</label><input class="input" id="setup-new-library-path" type="text" placeholder="输入新路径后会自动创建目录"></div>' +
        '</section>' +
      '</div>' +
      '<div class="setup-recovery-footer">' +
        '<button class="btn btn-primary" id="setup-select-library">切换资源库并重启</button>' +
        '<div class="login-error" id="setup-error"></div>' +
      '</div>';

    var selectedPath = '';
    Array.prototype.forEach.call(document.querySelectorAll('.setup-library-option'), function (btn) {
      btn.addEventListener('click', function () {
        selectedPath = btn.getAttribute('data-path') || '';
        Array.prototype.forEach.call(document.querySelectorAll('.setup-library-option'), function (item) {
          item.classList.toggle('active', item === btn);
        });
      });
    });

    document.getElementById('setup-select-library').addEventListener('click', async function () {
      var errorEl = document.getElementById('setup-error');
      errorEl.textContent = '';
      var newName = document.getElementById('setup-new-library-name').value.trim();
      var newPath = document.getElementById('setup-new-library-path').value.trim();
      var targetPath = newPath || selectedPath;
      if (!targetPath) {
        errorEl.textContent = '请先选择一个资源库，或填写新的资源库路径';
        return;
      }
      var btn = this;
      btn.disabled = true;
      btn.textContent = '切换中…';
      try {
        var resp = await fetch('/api/setup/select-library', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({name: newName, storage_path: targetPath})
        });
        var data = await resp.json();
        if (!resp.ok) throw new Error(data.error || '切换失败');
        btn.textContent = '正在重启…';
        errorEl.style.color = 'var(--accent)';
        errorEl.textContent = data.message || '服务正在重启';
        setTimeout(function () { location.reload(); }, 2000);
      } catch (e) {
        btn.disabled = false;
        btn.textContent = '切换资源库并重启';
        errorEl.style.color = 'var(--danger)';
        errorEl.textContent = e.message || '切换失败';
      }
    });
  }

  if (state.mode === 'library_recovery') renderRecovery();
  else renderInit();
})();
</script>
</body>
</html>`, payload)
}

func validateStartupLibrary(cfg *config.Config) error {
	info, err := os.Stat(cfg.StoragePath)
	if err != nil {
		return fmt.Errorf("当前资源库不可用: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("当前资源库不是目录: %s", filepath.Clean(cfg.StoragePath))
	}
	return nil
}
