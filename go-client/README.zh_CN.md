# XStreaming Go 预览版

这是使用 MyGo 和 Go 重写的早期预览，代码仍在开发中。微软账号登录、云游戏及 Xbox 主机串流流程、音视频和手柄输入尚未经过真实账号与设备的完整验证。原 Electron 版本仍位于仓库根目录。

## 下载与运行

在本仓库的 GitHub Actions 中选择成功的 **Go client** 工作流运行，从 Artifacts 下载对应平台的 `xstreaming-go-*` 文件。构建产物不包含 FFmpeg：请自行安装同时包含 `ffmpeg` 和 `ffplay` 的版本，并将它们加入 PATH。程序用 `ffmpeg` 解码视频、用 `ffplay` 播放音频。

Linux 和 macOS 解压后可能需要 `chmod +x xstreaming`，然后从终端运行 `./xstreaming`。Windows 运行 `xstreaming.exe`。这些产物是未经签名的预览二进制，尚未打包成安装程序。运行时还需要目标平台的 MyGo 图形环境，具体参见 [MyGo 文档](https://github.com/egoist/mygo)。

## 从源码构建

需要 Go 1.27.3 或更新版本以及 FFmpeg/ffplay。进入仓库的 `go-client` 目录：

```sh
go test ./...
go build -o xstreaming ./cmd/xstreaming
./xstreaming
```

Windows 上可运行 `go build -o xstreaming.exe ./cmd/xstreaming`，然后启动 `xstreaming.exe`。Linux 需要图形会话；读取手柄还需要访问对应 `/dev/input` 设备的权限。

## 当前范围

- 支持微软设备代码登录，以及系统凭据库可用时的登录恢复；可在界面中退出登录。系统凭据库不可用时只在当前运行期间保存登录。
- 支持查询主机、云游戏目录和尝试启动串流；云游戏取决于账号权益和地区。可搜索游戏并设置串流分辨率、游戏语言和目录市场；更改语言或市场后需要重新加载目录。
- 支持第一个手柄的基本输入，也可在串流时启用键盘手柄模式：WASD 为左摇杆、TFGH 为右摇杆、IJKL 为面键、方向键为方向键、1/4 为扳机、2/3 为肩键，Esc 退出键盘模式。尚不支持震动和多手柄。音视频同步、延迟和真实 Xbox 兼容性尚未验证。
- 目录使用系统的本地配置路径，登录凭据使用系统凭据库；FFmpeg/ffplay 是独立运行依赖，不随产物分发。

开发和测试说明另见 [英文说明](README.md)。
