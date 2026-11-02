# mygo-wallpaper


#### 安装教程

### 一键运行步骤
 
1. 安装 Go 1.27.3
​
- 官网下载 go1.27.3，配置GOROOT GOPATH
​
2. 安装 mygo 框架
 
 go install github.com/therecipe/qt/cmd/...@latest
go install github.com/ying32/govcl/vcl@latest
 
3. 初始化依赖
 
 go mod tidy

 4. 直接运行

go run main.go

5. 打包发布（跨平台编译）

# Linux amd64
GOOS=linux GOARCH=amd64 go build -o wallpaper
# 信创arm64（麒麟arm）
GOOS=linux GOARCH=arm64 go build -o wallpaper-arm64

# 构建deb包
dpkg-buildpackage -us -uc -b


#### Linux 运行时依赖

程序基于 govcl(Lazarus LCL)，运行时需要 liblcl.so（gtk2 版本）：

- deb 包已内置 liblcl.so（安装到 /usr/lib/<multiarch>/），并声明 GTK2 依赖：
  Debian 13 为 libgtk2.0-0t64，旧版本为 libgtk2.0-0。
- 手动编译运行时，需将对应架构的 liblcl.so 复制到可执行文件目录，
  或 /usr/lib/、/usr/lib/x86_64-linux-gnu/ 等系统库目录。
  amd64 下载：https://github.com/ying32/govcl/releases （liblcl-2.2.3.zip）
- 构建 deb 前需在源码根目录放置 liblcl.so（CI 会自动按架构下载）。


#### 参与贡献

1.  Fork 本仓库
2.  新建 Feat_xxx 分支
3.  提交代码
4.  新建 Pull Request
