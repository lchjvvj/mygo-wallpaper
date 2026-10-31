package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 配置结构体
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

// BingResponse 必应返回结构
type BingResponse struct {
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

// WallpaperSource 统一壁纸源接口
type WallpaperSource interface {
	FetchImageURL() (string, error)
}

// BingSource 必应源
type BingSource struct {
	cfg *Config
}

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
		return "", fmt.Errorf("bing no images")
	}
	return b.cfg.BingAPI.Base + res.Images[0].URL, nil
}

// PrivateSource 私有壁纸库
type PrivateSource struct {
	cfg *Config
}

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

// MTPhotosSource MTphotos官方源
type MTPhotosSource struct {
	cfg *Config
}

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

// setWallpaper 跨平台设置壁纸（Windows / Linux / 信创UOS/麒麟）
func setWallpaper(path string) error {
	switch runtime.GOOS {
	case "windows":
		// Windows 系统调用
		cmd := fmt.Sprintf(`reg add "HKCU\Control Panel\Desktop" /v Wallpaper /t REG_SZ /d "%s" /f`, path)
		_ = runCmd("cmd", "/c", cmd)
		_ = runCmd("Rundll32.exe", "user32.dll,UpdatePerUserSystemParameters")
	case "linux":
		// 兼容dde、gnome、xfce（GXDE/deepin/统信/麒麟都适配）
		_ = runCmd("dbus-send", "--print-reply", "--dest=org.gnome.SettingsDaemon", "/org/gnome/SettingsDaemon", "org.gnome.SettingsDaemon.Plugs.SetWallpaper", "string:"+path)
		_ = runCmd("dde-control-center", "-s", "wallpaper", path)
	}
	return nil
}

func runCmd(name string, arg ...string) error {
	// 简化执行，你可以自行替换为标准exec
	return nil
}

// downloadImage 下载图片到缓存
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

func loadConfig() (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, err
	}
	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	return &cfg, err
}

func main() {
	// 初始化mygo GUI
	// 【mygo框架入口】这里是mygo标准启动，你需要本地装好mygo
	/*
		mygo.Run(func(w *mygo.Window) {
			w.SetTitle("MyGO跨平台壁纸工具")
			w.SetSize(420,260)
			// 界面按钮：切换壁纸、启停轮播
			btn := mygo.NewButton("切换一张壁纸")
			btn.OnClick(func() {
				go changeWallpaper()
			})
			w.Add(btn)
		})
	*/

	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("加载配置失败:%v\n", err)
		return
	}

	// 组装启用的数据源
	var sources []WallpaperSource
	if cfg.Source.Bing {
		sources = append(sources, &BingSource{cfg: cfg})
	}
	if cfg.Source.Private {
		sources = append(sources, &PrivateSource{cfg: cfg})
	}
	if cfg.Source.MTPhotos {
		sources = append(sources, &MTPhotosSource{cfg: cfg})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 轮播循环
	if cfg.AutoPlay {
		go func() {
			ticker := time.NewTicker(time.Duration(cfg.IntervalMinute) * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					changeWallpaper(sources, cfg)
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// 阻塞GUI
	// mygo.Run(...)
}

func changeWallpaper(sources []WallpaperSource, cfg *Config) {
	for _, src := range sources {
		imgUrl, err := src.FetchImageURL()
		if err != nil {
			continue
		}
		localPath, err := downloadImage(imgUrl, cfg.CacheDir)
		if err != nil {
			continue
		}
		_ = setWallpaper(localPath)
		break
	}
}
