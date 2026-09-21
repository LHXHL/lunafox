# LunaFox × ChainReactors 引擎接入落地方案

> 分支：`feat/chainreactors-engines`（推送到 `LHXHL/lunafox`）
> 目标：将 gogo / spray / zombie 三个扫描引擎以第三方引擎扩展的形式接入 LunaFox，与官方引擎共存，通过自定义工作流在扫描中选用。

---

## 1. 目标与范围

| 引擎 | 替代/新增 | 职责 |
|---|---|---|
| `engine.lhxhl.gogo` | 替代 ports + websites 两阶段（naabu + website_discovery），并顺带产出指纹 | 端口扫描 + 服务识别 + 指纹 + POC，一次执行四路结果 |
| `engine.lhxhl.spray` | 替代 directory_scan（ffuf） | 目录爆破 + 单页信息 + 敏感信息 |
| `engine.lhxhl.zombie` | 新增能力（无官方对应） | 服务口令爆破（弱口令即漏洞发现） |

**不做的事**：不改 Server/contracts 源码；不动闭源 Agent；不卸载官方引擎（共存，工作流级选用）。

## 2. 已验证的平台事实（源码级）

1. **扩展点全部开放**：每个引擎的生成契约无条件暴露 4 种 fact 输入（`Subdomains`/`HostPorts`/`WebsiteURLs`/`EndpointURLs`，挂载于 `/run/lunafox/inputs/`）与 8 种规范结果类型的提交端口（惰性创建）。
2. **生成器缺失但不阻塞**：`tools/engine-manifest-artifact-gen` 未导出；对比 directory_scan 与 port_scan 的生成文件，**唯一差异是 Config 结构体**（与 engine.json 的 `execution.configSections` 一一映射）。方案：整目录复制 directory_scan 生成文件，手改 Config 段。
3. **结果 schema 校验约束**（写入 `contracts/results/，提交时 fail-closed`）：
   - `asset.vulnerability.v1` 的 `URL` **必须以 `http://` 或 `https://` 开头** → 非 Web 服务（ssh/mysql/redis…）的漏洞/弱口令，URL 统一构造为 `http://<ip>:<port>`，真实 scheme/服务/凭据进 `vulnType` 与 `rawOutput`。
   - `severity` 枚举：`unknown/info/low/medium/high/critical`；gogo 的整数 severity（1=info,2=medium,3=high,4=critical,5=unknown）直接映射。
   - `rawOutput` 必填（map），用于保留工具原始证据。
4. **安装约束**：安装引用必须是 `registry/repo@sha256:<digest>`（匿名 HTTPS OCI）；production 要求 Runtime Image Index 同时含 `linux/amd64` + `linux/arm64` 描述符；第三方包**无需官方签名**（通用安装器不解释发布者签名）。
5. **工作流约束**：工作流 JSON 由 Server 启动时从 `WORKFLOW_DEFINITIONS_ROOT` 加载；引用未安装引擎 → Server 拒绝启动（先装引擎再挂工作流）。

## 3. 工具侧事实（源码级，含锁定版本）

| 工具 | 版本 | 二进制来源（锁定 SHA-256） |
|---|---|---|
| gogo | v2.15.0 | release `gogo_linux_amd64` `3b51080a…db90` / `gogo_linux_arm64` `f34e248b…20a9` |
| spray | v0.3.2 | release `spray_linux_amd64` `5e6c7787…63e8` / `spray_linux_arm64` `907185fc…d03d` |
| zombie | v1.3.0 | release `zombie_linux_amd64` `94cc277b…6325` / `zombie_linux_arm64` `0bb30589…4c7` |

**输出格式（引擎解析器依据，均来自源码结构体定义）**：

- gogo `-o jl` → JSONL，每行 `GOGOResult`：`ip, port, protocol, status, uri?, host?, title?, midware?, timing?, frameworks{name→{name,tags?,…}}, vulns{name→{name,severity(int),payload?,detail?}}, extracted?`
- spray `-f <file> -O json` → JSONL（仅 valid/fuzzy 行），每行 `SprayResult`：`url, path, host, status(int), body_length, header_length, redirect_url?, content_type, title, frameworks, extracts, spend(ms), source, depth`
- zombie `-f <file> -O json` → JSONL（仅 `ok:true` 行），每行内联 `ZombieResult`：`ip, port, service, username, password, scheme, ok, error?, vulns?, loot?`
- zombie 支持服务（插件目录枚举）：ftp ssh smb mssql mysql redis rdp pop3 postgre mongo oracle memcache ldap mq rsync snmp socks5 vnc zookeeper http 等
- 三个工具均为 GPL-3.0，纯 Go 静态二进制，官方 release 已覆盖 linux amd64/arm64。

## 4. 引擎设计

### 4.1 `extensions/engines/gogo/` — engine.lhxhl.gogo

- **目标类型**：`domain, ip, cidr`
- **输入**：`Subdomains` fact + `Execution.Target` 基线（对齐 port_scan 模式：候选 = Target 展开 + 事实文件逐行合并）
- **configSections**（节 id `gogo`）：

| param | 类型 | 默认 | 约束 |
|---|---|---|---|
| `port-tag` | string | `top2` | enum: top1/top2/top3/common/all/… |
| `ports` | string | `` | 手动端口范围（空=用 port-tag） |
| `concurrency` | integer | 600 | 1–4000 |
| `timeout` | integer | 3 | 1–30（秒） |
| `smart-mode` | enum | `smart` | none/smart/supersmart |
| `exploit` | bool | true | 是否启用 POC 检测 |

- **命令行**：`gogo -i @candidates.txt -p <tag|ports> -t <concurrency> -d <timeout> -m <mode> [--exploit] -o jl -f /workspace/gogo.jsonl -q`
- **结果映射**（每行 JSONL 分流）：

| gogo 字段 | LunaFox 结果 | 映射 |
|---|---|---|
| 每行（开放端口） | `HostPort` | `{Host: host\|ip, IP: ip, Port: port}` |
| `protocol` http/https 且有 uri | `Website` | `{URL: protocol://ip:port uri, Host, Title, StatusCode: status, Webserver: midware}` |
| `frameworks` 非空 | `WebsiteTechnology` | `{URL, Tech: [names…]}` |
| `vulns` 每项 | `Vulnerability` | `{URL: http(s)://ip:port（非 http 协议兜底 http://ip:port）, VulnType: name, Severity: 枚举映射, Source: "gogo", RawOutput: {ip,port,protocol,payload…}}` |

### 4.2 `extensions/engines/spray/` — engine.lhxhl.spray

- **目标类型**：`domain, ip, cidr`
- **输入**：`WebsiteURLs` fact（照抄 directory_scan 的消费模式）
- **configSections**（节 id `spray`）：

| param | 类型 | 默认 | 约束 |
|---|---|---|---|
| `wordlist` | string(resource: wordlist) | `dir_default.txt` | 平台字典资源 |
| `rule` | string(resource: wordlist) | 空 | hashcat 风格规则文件（可选） |
| `concurrency` | integer | 20 | 1–200 |
| `timeout` | integer | 604800 | 执行总超时（秒）60–604800 |
| `exclude-subtract` | bool | true | 智能过滤动态目录 |
| `resume` | bool | false | 断点续传（workspace 内 stat.json） |

- **命令行**：`spray -l candidates.txt -d <wordlist> [-r rule] -f /workspace/spray.jsonl -O json -q`（并发等经环境/参数透传）
- **结果映射**：

| spray 字段 | LunaFox 结果 | 映射 |
|---|---|---|
| `valid` 且 status 非基准 | `Directory` | `{URL, Status, ContentLength: body_length, ContentType, Duration: spend(ms)}` |
| `frameworks` 非空 | `WebsiteTechnology` | `{URL, Tech}` |
| `extracts` 敏感信息 | `Endpoint`（title 携带提取摘要） | 保底映射，数量限流 |

### 4.3 `extensions/engines/zombie/` — engine.lhxhl.zombie

- **目标类型**：`ip, cidr`
- **输入**：`HostPorts` fact（`host-ports.jsonl`，每行 `{host,ip,port}`）——本生态第一个 HostPorts 消费者；按端口→服务推断（内置常见端口映射表 + 参数覆盖），**仅保留 zombie 支持的服务**
- **configSections**（节 id `zombie`）：

| param | 类型 | 默认 | 约束 |
|---|---|---|---|
| `services` | string[] | `["ssh","mysql","redis","mssql","ftp","smb","rdp","postgre"]` | enum: zombie 插件全集 |
| `username-wordlist` | string(resource) | 内置空 | 用户名字典（可选） |
| `password-wordlist` | string(resource) | 内置空 | 密码字典（可选） |
| `weakpass` | bool | false | 启用弱口令规则生成 |
| `concurrency` | integer | 50 | 1–500 |
| `timeout` | integer | 5 | 1–60（单次认证秒） |

- **命令行**：逐服务调用 `zombie -i <ip> -p <port> -s <service> [-U users -P pwds | --weakpass] -t <concurrency> -d <timeout> -f /workspace/zombie-<svc>.jsonl -O json -q`
- **结果映射**：`ok:true` 行 → `Vulnerability{URL: "http://<ip>:<port>"（平台 URL 约束的既定取舍）, VulnType: "weak-credential:<service>", Severity: "critical", Source: "zombie", Description: "…", RawOutput: {service, scheme, username, password, loot…}}`

### 4.4 每个引擎的文件清单（以 spray 为例，其余同构）

```
extensions/engines/spray/
├── engine.json                      # engine.v5 清单（唯一事实源）
├── go.mod / go.sum                  # module github.com/yyhuni/lunafox/engines/spray, replace engine-go
├── Dockerfile                       # 三段式：工具下载(锁定checksum) → 引擎构建 → runtime+conformance
├── README.md
├── contract/execution_generated.go  # 复制自 directory_scan，仅手改 Config 段
├── cmd/spray-engine/
│   ├── main.go                      # Run(runEngine)
│   └── execution_run_generated.go   # 复制自 directory_scan，改包引用
├── runtime/
│   ├── execution.go                 # 生命周期：校验→物化输入→候选→执行→解析→提交
│   ├── candidates.go                # fact 输入展开 + Target 基线
│   ├── command.go                   # CLI 构造（纯函数，可测）
│   ├── parser.go                    # JSONL 解析 → 规范结果（纯函数，fixture 测试）
│   └── process.go                   # 子进程执行（照抄 container_execution 模式）
├── locales/en.json, zh.json
└── tests/container/{container-conformance.sh, image-conformance.json, tool-stubs/}
```

### 4.5 工作流 `extensions/workflows/chainreactors.scan-workflow.json`

```json
{"scanWorkflowId": "chainreactors", "displayName": "ChainReactors Scan",
 "stages": [
  {"stageId":"discovery","steps":[{"stepId":"subdomain_discovery","engineId":"engine.lunafox.subdomain_discovery","profileDefaultEnabled":true}]},
  {"stageId":"recon","steps":[{"stepId":"gogo","engineId":"engine.lhxhl.gogo","profileDefaultEnabled":true}]},
  {"stageId":"content","steps":[
    {"stepId":"url_collection","engineId":"engine.lunafox.url_collection","profileDefaultEnabled":true},
    {"stepId":"spray","engineId":"engine.lhxhl.spray","profileDefaultEnabled":true}]},
  {"stageId":"screenshot","steps":[{"stepId":"screenshot","engineId":"engine.lunafox.screenshot","profileDefaultEnabled":true}]},
  {"stageId":"nuclei","steps":[{"stepId":"nuclei_vulnerability","engineId":"engine.lunafox.nuclei_vulnerability","profileDefaultEnabled":false}]},
  {"stageId":"credential","steps":[{"stepId":"zombie","engineId":"engine.lhxhl.zombie","profileDefaultEnabled":false}]}
]}
```

gogo 单阶段吃掉官方 ports+websites 两阶段；zombie 默认关闭（侵入性，显式开启）。

## 5. 实施步骤

1. **骨架生成**：以 directory_scan 为模板复制三份，改模块路径/二进制名/Config 段，`go mod tidy` 解析依赖。
2. **runtime 实现**：按 4.1–4.3 写 candidates/command/parser/process；每个引擎的 parser 与 command 为纯函数并带表驱动单测（fixture 来自第 3 节源码结构）。
3. **Dockerfile**：工具下载段按锁定 checksum 双架构下载（对齐 ffuf 模式）；引擎构建段复用 `--build-context contracts/engine-go`；runtime 段安装 `/opt/lunafox-tools/bin/<tool>` + VERSIONS；conformance 段验证二进制存在且引擎入口可执行。
4. **工作流 JSON** + locales。
5. **本地验证（Tier 1，本次会话完成）**：
   - 每引擎 `go build ./... && go vet ./... && go test ./...`
   - `docker buildx build --platform linux/amd64,linux/arm64` 至少一个引擎完整构建通过
   - engine.json JSON 语法 + 与 Config 结构体字段一致性人工核对清单
6. **部署验证（Tier 2，需要运行环境，本方案交付后执行）**：
   - dev compose 引擎安装链：本地 registry → package publisher 产出 `.lfengine.tar.gz` → bootstrap 注册
   - 生产安装：UI 粘贴 `docker.io/lhxhl/lunafox-engine-runtime-<name>@sha256:…`
   - 挂载 chainreactors 工作流 → 对授权测试目标发起扫描 → 前端确认四类资产入库
7. **推送**：`feat/chainreactors-engines` → `LHXHL/lunafox`。

## 6. 风险与既定取舍

| 风险 | 处置 |
|---|---|
| codegen 未导出，手改 Config 段可能与 engine.json 漂移 | engine.json 为唯一事实源；README 记录同步规则；字段命名机械映射（`kebab-case` → `CamelCase`） |
| Vulnerability URL 强制 http(s) | 既定取舍：非 Web 服务用 `http://ip:port` 定位，真实信息在 vulnType/rawOutput |
| zombie 端口→服务推断不准 | 内置常见端口映射 + `services` 参数白名单过滤；无匹配则跳过并 Progress 说明 |
| 打包 media type 严格 fail-closed | Tier 2 首次打包预留调试时间；直接复用仓库 publisher 工具链 |
| 引擎 JSONL 巨量结果 | 去重（URL 精确去重）+ 分批 Submit（ExecutionLimits 由平台限流） |
| GPL-3.0 | wrapper 同 GPL；自用无义务，对外分发需开源 wrapper |
| 爆破类能力合规 | zombie Profile 默认关闭；仅用于授权目标 |

## 8. 验证结果

### Tier 1（本地，已完成）

| 门 | 结果 |
|---|---|
| 三引擎 `go build ./...` + `go vet ./...` | ✅ 通过 |
| 三引擎单元测试（命令构造 + 解析映射 + 目标过滤，表驱动） | ✅ 全绿 |
| engine.json 严格解码（复用平台 `contracts/enginemanifest.DecodeRootManifest`） | ✅ 三个清单全部通过 |
| 工作流 JSON 严格校验（复用 `contracts/scanworkflow.DecodeDefinition + ValidateDefinition`） | ✅ default + chainreactors 均通过 |
| Docker 镜像构建（工具下载 SHA256 校验 → 容器内编译 → conformance 门） | ✅ 三镜像全部通过 |
| 镜像内真实工具运行（gogo -P port / spray --help / zombie -l） | ✅ 全部正常输出 |

### Tier 2（本地完整部署，已完成）

在本地以开发模式跑通了完整生产链路：本地 OCI registry（HTTP:5500，macOS 的 5000 被 AirPlay 占用）
→ 三引擎 runtime image（arm64，Docker manifest 经 registry API 转换为符合验证器要求的
单平台 OCI index）→ `.lfengine.tar.gz`（平台 `enginepackagebuild` 生成）→ `engine-oci-publish`
发布为 OCI artifact → compose 引导安装（`ENGINE_INSTALL_DEVELOPMENT_MODE=true`，bootstrap 补丁
脚本跳过与开发模式互斥的 `ENGINE_INSTALL_REGISTRY` 断言）→ API 注册可见 → 工作流加载 →
对 nginx 测试目标（192.168.163.10）端到端扫描。

**端到端结果（scan 5，status=succeeded，8 秒）**：

| 任务 | 引擎 | 结果 |
|---|---|---|
| recon | gogo | `records=1 hostPorts=1 websites=1 technologies=1`（nginx/80 指纹命中） |
| content | spray | 消费 gogo 产出的 WebsiteURLs 事实 → `directories=1 technologies=1` |
| credential | zombie | 消费 HostPorts 事实 → `targets=0 skippedPorts=1`（80 端口无服务映射，按设计优雅跳过） |

数据库证据：`website` 表 `http://192.168.163.10:80/ | Welcome to nginx!`；`host_port_mapping`
表 `192.168.163.10:80`；`directory` 表 `url=/ status=200 content_length=896`。

### Tier 2 过程中发现并修复的问题

1. **gogo jl 输出首行是参数回显**（无 port 字段）→ 解析器跳过无 port 行（真实容器执行中发现）。
2. **spray 的 `extracts` 字段是数组而非对象** → 解析器改用 `json.RawMessage`（真实输出夹具入测试）。
3. **生成适配器吞掉引擎错误**（只打印固定失败消息）→ 三个引擎 main.go 增加错误透传到 stderr。
4. **工作流 ID 不允许连字符**（`^[a-z][a-z0-9_]{0,63}$`）→ dev 工作流命名 `chainreactors_dev`。
5. **quickCreate 的 wordlist 参数必须用资源名**（`wordlists/1`），profile 草稿返回的文件名
   `dir_default.txt` 不能直接提交。

### 重要平台事实修正（生产部署须知）

**生产模式（`ENGINE_INSTALL_DEVELOPMENT_MODE` 未开启）下，`POST /v1/engines:install`
对每个非 CF 候选强制执行 Sigstore keyless 验签**（`infra.go` 始终构造
`NewProductionSigstoreKeylessVerifier`，验证目标为 `yyhuni/lunafox` 仓库的 GitHub OIDC 身份；
无签名 referrer 时返回 `MissingBundleError` 并 fail-closed）。因此：

- 第三方引擎包在生产模式下**无法通过 operator API 安装**（此前基于 README 网页摘要的
  "签名不决定准入"结论仅适用于模块契约层，实际生产装配层强制验签）。
- 自托管第三方引擎的现实路径：`ENGINE_INSTALL_DEVELOPMENT_MODE=true`（需同时启用
  plain HTTP 本地 registry 与单平台镜像，bootstrap inventory 安装），或 fork server
  放宽验签器装配。
- operator API 路径即使开发模式也要求匿名 HTTPS registry + 双架构 index（`infra.go`
  未把 dev 策略接到 operator puller）；本地第三方安装走 bootstrap inventory 路径。

### 生产部署清单（后续真实上线时）

1. 双架构镜像（`docker buildx build --platform linux/amd64,linux/arm64`）
2. 发布到匿名可拉的 HTTPS registry（如 ghcr.io）
3. 若保持平台生产模式：需 fork server 移除/放宽 `infra.go` 的验签器装配（GPL 允许）
4. `extensions/workflows/chainreactors.scan-workflow.json` 需在官方 8 引擎已安装的环境使用
   （其引用了官方 subdomain_discovery/url_collection/screenshot/nuclei 引擎）

## 9. 回滚

- 引擎安装幂等：同 engineId 换 digest 需 `allowReplacement=true`，删除引擎记录即回滚。
- 工作流文件移除后 Server 重启即回到官方 default 工作流。
- 分支不合并 main 则对主仓库零影响。
