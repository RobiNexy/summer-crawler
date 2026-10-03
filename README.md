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
