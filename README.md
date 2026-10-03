# Summer 爬虫

用于抓取 Summer 平台动态及好友资料的 Go 命令行工具。

授权令牌不会写入源代码。编译并运行：

```bash
go build -o summer_crawl .
./summer_crawl 'your-api-key'
```

也可以直接从 [Releases](../../releases) 下载对应平台的压缩包（Linux / macOS / Windows，含二进制、模板和说明文档），解压后运行其中的 `summer_crawl`（Windows 为 `summer_crawl.exe`）。

本次只抓少量动态进行试跑：

```bash
./summer_crawl 'your-api-key' -sample 3
```

调整并行度（默认 4，好友详细资料与资源下载会并行执行）：

```bash
./summer_crawl 'your-api-key' -concurrency 8
```

启动后会交互询问是否保留黑板墙动态和好友信息。默认输出到 `summer_crawl_output/`，也可以指定目录：

```bash
./summer_crawl 'your-api-key' -output-dir ./data
```

程序会生成动态清洗结果：

- `memories.json`：全部原始动态。
- `normal_memories.json`：普通动态。
- `refined_normal_memories.json`：提炼后的标题、正文、图片、时间及结构化外链。
- `memories_for_llm.md`：普通动态的时间和正文 Markdown 汇总。
- `blackboard_memories.json`、`refined_blackboard_memories.json`：选择保留黑板墙时生成。
- `friends.json`：选择保留好友信息时生成。
- `profile.json`：个人资料。
- `paper.json`：个人交友问卷。
- `question_boards.json`：自己发布过的黑板墙问题及回答。
- `album.html`：可直接打开的离线 HTML 相册。

相册包含年份导航、两列动态卡片、图片灯箱、评论区、录音播放器、好友勋章墙和个人资料页。下载的资源位于 `assets/images/avatar/`、`assets/images/memories/`、`assets/images/questions/` 和 `assets/audio/`，不依赖网络加载 JavaScript 框架。

列表接口每次请求 100 条（动态为 200 条），分页大小会自动适配服务端上限。所有 API 请求和资源下载之间会加入 0.1 至 0.3 秒随机等待；好友详细资料与资源下载默认以 4 个并发执行，可用 `-concurrency` 调整。

## 抓包获取 key

程序第一个参数就是请求中的 `Authorization` 值。推荐用 mitmproxy/mitmweb 抓取：

```bash
mitmweb -w summer.mitm
```

在 mitmweb 中找到 Summer 的任意 API 请求，在请求头里复制 `authorization` 的值。不要复制 `checksum`、`nonce` 等临时字段。抓取完成后，保存 `.mitm` 文件，后续分析接口和媒体字段时可以直接使用。

### Android

1. 手机和电脑连接同一个 Wi-Fi，查看电脑局域网 IP。
2. 手机 Wi-Fi 代理选择“手动”，填写电脑 IP 和 mitmproxy 端口（默认 `8080`）。
3. 手机浏览器访问 `http://mitm.it`，下载 Android 证书并安装。
4. Android 7 及以上系统可能不允许普通应用信任用户证书；若 Summer 不出现在抓包列表，通常需要使用已解锁设备/系统证书，或使用应用自带的调试环境。
5. 打开 Summer，回到 mitmweb 查看请求头中的 `authorization`。

### iPhone

1. iPhone 和电脑连接同一个 Wi-Fi，Wi-Fi 代理手动填写电脑 IP 和 `8080` 端口。
2. Safari 访问 `http://mitm.it`，下载 iOS 证书。
3. 在“设置 → 已下载的描述文件”安装证书。
4. 再到“设置 → 通用 → 关于本机 → 证书信任设置”开启对 mitmproxy 证书的完全信任。
5. 打开 Summer，在 mitmweb 中复制 API 请求头的 `authorization`。

如果抓不到 HTTPS 请求，先确认手机代理仍指向电脑、证书已信任，并检查应用是否使用了证书绑定（certificate pinning）。不要把包含 `authorization`、Cookie 或个人数据的抓包文件上传到 Git；本项目已忽略 `API/` 和生成输出目录。
