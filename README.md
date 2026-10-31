# bing-wallpaper


#### 安装教程

### 一键运行步骤
 
1. 安装 Go 1.27.3
​
- 官网下载 go1.27.3，配置GOROOT GOPATH
​
2. 安装 mygo 框架
 
 go install github.com/therecipe/qt/cmd/...@latest
 
3. 初始化依赖
 
 go mod tidy

 4. 直接运行

go run main.go

5. 打包发布（跨平台编译）

# Windows
GOOS=windows GOARCH=amd64 go build -o wallpaper.exe
# Linux / 信创amd64
GOOS=linux GOARCH=amd64 go build -o wallpaper
# 信创arm64（麒麟arm）
GOOS=linux GOARCH=arm64 go build -o wallpaper-arm64


#### 参与贡献

1.  Fork 本仓库
2.  新建 Feat_xxx 分支
3.  提交代码
4.  新建 Pull Request