package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"github.com/ying32/govcl/vcl"
	"github.com/ying32/govcl/vcl/types"
)

// 注意：这里用的是社区主流 mygo = govcl（LCL 跨平台GUI，信创/Windows/Linux通用）
// 如果你是别的mygo分支，我可以再替换对应控件API

type Config struct {
	AutoPlay       bool   `yaml:"auto_play"`
	IntervalMinute int    `yaml:"interval_minute"`
	CacheDir       string `yaml:"cache_dir"`
	MaxCacheNum    int    `yaml:"max_cache_num"`
	Source         struct {
		Bing     bool `yaml:"bing"`
		Private  bool `yaml:"private"`
		MTPhotos bool `yaml:"mtphotos"`
	} `yaml:"source"`
	BingAPI struct {
		URL  string `yaml:"url"`
		Base string `yaml:"base"`
	} `yaml:"bing_api"`
	PrivateAPI struct {
		URL    string `yaml:"url"`
		Token  string `yaml:"token"`
		Method string `yaml:"method"`
	} `yaml:"private_api"`
	MTPhotosAPI struct {
		URL   string `yaml:"url"`
		Token string `yaml:"token"`
	} `yaml:"mtphotos_api"`
}

var (
	globalCfg *Config
	ticker    *time.Ticker
	ctxPlay   context.Context
	cancelPlay context.CancelFunc
	mu        sync.Mutex
)

type BingResponse struct {
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

type WallpaperSource interface {
	FetchImageURL() (string, error)
	Name() string
}

type BingSource struct{ cfg *Config }
func (b *BingSource) Name() string { return "Bing必应" }
func (b *BingSource) FetchImageURL() (string, error) {
	resp, err := http.Get(b.cfg.BingAPI.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var res BingResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	if len(res.Images) == 0 {
		return "", fmt.Errorf("bing返回空图片列表")
	}
	return b.cfg.BingAPI.Base + res.Images[0].URL, nil
}

type PrivateSource struct{ cfg *Config }
func (p *PrivateSource) Name() string { return "私有壁纸库" }
func (p *PrivateSource) FetchImageURL() (string, error) {
	req, err := http.NewRequest(p.cfg.PrivateAPI.Method, p.cfg.PrivateAPI.URL, nil)
	if err != nil {
		return "", err
	}
	if p.cfg.PrivateAPI.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.PrivateAPI.Token)
	}
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		ImgURL string `json:"img_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ImgURL, nil
}

type MTPhotosSource struct{ cfg *Config }
func (m *MTPhotosSource) Name() string { return "MTphotos" }
func (m *MTPhotosSource) FetchImageURL() (string, error) {
	req, err := http.NewRequest("GET", m.cfg.MTPhotosAPI.URL, nil)
	if err != nil {
		return "", err
	}
	if m.cfg.MTPhotosAPI.Token != "" {
		req.Header.Set("X-Token", m.cfg.MTPhotosAPI.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func loadConfig() (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, err
	}
	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	return &cfg, err
}

func saveConfig(cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile("config.yaml", data, 0644)
}

func setWallpaper(path string) error {
	mu.Lock()
	defer mu.Unlock()
	switch runtime.GOOS {
	case "windows":
		cmd1 := exec.Command("reg", "add", `HKCU\Control Panel\Desktop`, "/v", "Wallpaper", "/t", "REG_SZ", "/d", path, "/f")
		_ = cmd1.Run()
		cmd2 := exec.Command("Rundll32.exe", "user32.dll,UpdatePerUserSystemParameters")
		_ = cmd2.Start()
	case "linux":
		// GXDE / Deepin DDE 优先：通过gsettings写入deepin兼容schema，由dde-daemon生效
		absPath, absErr := filepath.Abs(path)
		if absErr != nil {
			absPath = path
		}
		_ = exec.Command("gsettings", "set", "com.deepin.wrap.gnome.desktop.background", "picture-uri", "file://"+absPath).Start()
		// 兼容gnome fallback
		_ = exec.Command("dbus-send", "--print-reply", "--dest=org.gnome.SettingsDaemon", "/org/gnome/SettingsDaemon", "org.gnome.SettingsDaemon.Plugs.SetWallpaper", "string:"+path).Start()
	}
	return nil
}

func downloadImage(url string, cacheDir string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_ = os.MkdirAll(cacheDir, 0755)
	fname := filepath.Join(cacheDir, fmt.Sprintf("%d.jpg", time.Now().UnixNano()))
	f, err := os.Create(fname)
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return fname, err
}

func getActiveSources() []WallpaperSource {
	var list []WallpaperSource
	if globalCfg.Source.Bing {
		list = append(list, &BingSource{cfg: globalCfg})
	}
	if globalCfg.Source.Private {
		list = append(list, &PrivateSource{cfg: globalCfg})
	}
	if globalCfg.Source.MTPhotos {
		list = append(list, &MTPhotosSource{cfg: globalCfg})
	}
	return list
}

func changeOneWallpaper(logMemo func(string), previewSet func(string)) {
	srcs := getActiveSources()
	if len(srcs) == 0 {
		logMemo("❌ 没有启用任何壁纸数据源")
		return
	}
	for _, s := range srcs {
		logMemo(fmt.Sprintf("🔄 尝试从【%s】拉取壁纸", s.Name()))
		imgUrl, err := s.FetchImageURL()
		if err != nil {
			logMemo(fmt.Sprintf("❌ %s 拉取失败：%v", s.Name(), err))
			continue
		}
		local, err := downloadImage(imgUrl, globalCfg.CacheDir)
		if err != nil {
			logMemo(fmt.Sprintf("❌ 下载失败：%v", err))
			continue
		}
		logMemo(fmt.Sprintf("✅ 图片缓存：%s", local))
		previewSet(local)
		_ = setWallpaper(local)
		logMemo("✅ 壁纸设置完成")
		return
	}
	logMemo("⚠️ 所有数据源全部失败")
}

func startAutoPlay(logMemo func(string)) {
	if globalCfg.AutoPlay == false {
		return
	}
	if cancelPlay != nil {
		cancelPlay()
	}
	ctxPlay, cancelPlay = context.WithCancel(context.Background())
	ticker = time.NewTicker(time.Duration(globalCfg.IntervalMinute) * time.Minute)
	logMemo(fmt.Sprintf("▶️ 轮播开启，间隔：%d 分钟", globalCfg.IntervalMinute))
	go func() {
		for {
			select {
			case <-ticker.C:
				changeOneWallpaper(logMemo, func(s string) {})
			case <-ctxPlay.Done():
				ticker.Stop()
				logMemo("⏹️ 轮播已停止")
				return
			}
		}
	}()
}

func stopAutoPlay(logMemo func(string)) {
	if cancelPlay != nil {
		cancelPlay()
		cancelPlay = nil
		ticker = nil
	}
	logMemo("⏹️ 手动停止轮播")
}

func main() {
	vcl.Application.Initialize()
	vcl.Application.SetMainFormOnTaskBar(true)

	mainForm := vcl.Application.CreateForm()
	mainForm.SetCaption("MyGO跨平台壁纸工具｜Bing+私有库+MTphotos")
	mainForm.SetWidth(620)
	mainForm.SetHeight(520)

	// 控件
	chkBing := vcl.NewCheckBox(mainForm)
	chkBing.SetParent(mainForm)
	chkBing.SetBounds(20, 20, 120, 24)
	chkBing.SetCaption("启用必应Bing")

	chkPrivate := vcl.NewCheckBox(mainForm)
	chkPrivate.SetParent(mainForm)
	chkPrivate.SetBounds(160, 20, 140, 24)
	chkPrivate.SetCaption("启用私有壁纸库")

	chkMT := vcl.NewCheckBox(mainForm)
	chkMT.SetParent(mainForm)
	chkMT.SetBounds(320, 20, 130, 24)
	chkMT.SetCaption("启用MTphotos")

	labInterval := vcl.NewLabel(mainForm)
	labInterval.SetParent(mainForm)
	labInterval.SetBounds(20, 60, 100, 22)
	labInterval.SetCaption("轮播间隔(分钟):")

	edtInterval := vcl.NewEdit(mainForm)
	edtInterval.SetParent(mainForm)
	edtInterval.SetBounds(130, 58, 80, 24)

	btnSwitch := vcl.NewButton(mainForm)
	btnSwitch.SetParent(mainForm)
	btnSwitch.SetBounds(20, 100, 140, 32)
	btnSwitch.SetCaption("🔄 切换一张壁纸")

	btnStart := vcl.NewButton(mainForm)
	btnStart.SetParent(mainForm)
	btnStart.SetBounds(180, 100, 120, 32)
	btnStart.SetCaption("▶️ 开启轮播")

	btnStop := vcl.NewButton(mainForm)
	btnStop.SetParent(mainForm)
	btnStop.SetBounds(320, 100, 120, 32)
	btnStop.SetCaption("⏹️ 停止轮播")

	imgPreview := vcl.NewImage(mainForm)
	imgPreview.SetParent(mainForm)
	imgPreview.SetBounds(20, 150, 400, 220)
	imgPreview.SetStretch(true)

	memoLog := vcl.NewMemo(mainForm)
	memoLog.SetParent(mainForm)
	memoLog.SetBounds(20, 390, 560, 90)
	memoLog.SetReadOnly(true)

	logFn := func(msg string) {
		vcl.ThreadSync(func() {
			memoLog.Lines().Add(time.Now().Format("[15:04:05] ") + msg)
		})
	}
	previewFn := func(p string) {
		vcl.ThreadSync(func() {
			imgPreview.Picture().LoadFromFile(p)
		})
	}

	// 加载配置
	var err error
	globalCfg, err = loadConfig()
	if err != nil {
		logFn("⚠️ 读取config.yaml失败，新建默认配置")
		globalCfg = &Config{
			AutoPlay: false,
			IntervalMinute:30,
			CacheDir: "./cache",
			MaxCacheNum:50,
		}
		_ = saveConfig(globalCfg)
	}
	// 绑定配置到控件
	chkBing.SetChecked(globalCfg.Source.Bing)
	chkPrivate.SetChecked(globalCfg.Source.Private)
	chkMT.SetChecked(globalCfg.Source.MTPhotos)
	edtInterval.SetText(fmt.Sprintf("%d", globalCfg.IntervalMinute))

	// 切换壁纸按钮
	btnSwitch.SetOnClick(func(sender vcl.IObject) {
		go changeOneWallpaper(logFn, previewFn)
	})

	btnStart.SetOnClick(func(sender vcl.IObject) {
		globalCfg.Source.Bing = chkBing.Checked()
		globalCfg.Source.Private = chkPrivate.Checked()
		globalCfg.Source.MTPhotos = chkMT.Checked()
		fmt.Sscan(edtInterval.Text(), &globalCfg.IntervalMinute)
		globalCfg.AutoPlay = true
		_ = saveConfig(globalCfg)
		startAutoPlay(logFn)
	})

	btnStop.SetOnClick(func(sender vcl.IObject) {
		globalCfg.AutoPlay = false
		_ = saveConfig(globalCfg)
		stopAutoPlay(logFn)
	})

	mainForm.SetOnClose(func(sender vcl.IObject, action *types.TCloseAction) {
		stopAutoPlay(logFn)
	})

	vcl.Application.Run()
}
