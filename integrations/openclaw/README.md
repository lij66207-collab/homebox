# Homebox × OpenClaw（龙虾）集成

让 OpenClaw（中文昵称"龙虾"，前身为 Clawdbot / Moltbot）通过 Homebox REST API 帮你管理家庭库存：说一句话就能录入物品（支持生产日期、保质期、到期日），也能随时查询临期物品。

本目录内容：

- `SKILL.md` — OpenClaw 技能定义文件，安装后龙虾即可按其中的指令调用 Homebox API。
- `README.md`（本文件）— 安装与配置指南。

## 一、创建 Homebox API Token

1. 打开 Homebox 网页 UI 并登录。
2. 进入 **个人资料（Profile）** 页面，找到 **API Keys / API 令牌** 一栏。
3. 点击创建，给令牌起个名字（例如 `openclaw`），**立即复制生成的令牌**——它只完整显示这一次。

令牌对应的接口是 `POST /api/v1/users/self/api-keys`；调用 API 时通过 `Authorization: Bearer <令牌>` 请求头鉴权（API 令牌只接受 Authorization 头方式，放在 query 参数里会被拒绝）。

## 二、在 OpenClaw 中配置环境变量

在 OpenClaw 的配置（环境变量或 secrets 管理）中添加：

```bash
HBOX_BASE_URL=http://localhost:7745   # 你的 Homebox 地址；远程部署就填域名，如 https://homebox.example.com
HBOX_API_TOKEN=粘贴上一步复制的令牌
```

注意 `HBOX_BASE_URL` 不要以 `/` 结尾，也不要带 `/api` 后缀。如果 Homebox 和 OpenClaw 跑在同一台机器上，`http://localhost:7745` 即可。

## 三、安装技能

把 `SKILL.md` 放进 OpenClaw 的技能目录（复制或软链接均可）：

```bash
# 方式一：复制
cp integrations/openclaw/SKILL.md ~/.openclaw/skills/homebox/SKILL.md

# 方式二：软链接（仓库更新后技能同步更新）
mkdir -p ~/.openclaw/skills/homebox
ln -s "$(pwd)/integrations/openclaw/SKILL.md" ~/.openclaw/skills/homebox/SKILL.md
```

技能目录的具体位置以你的 OpenClaw 版本为准（通常在 `~/.openclaw/skills/` 下，每个技能一个子目录）。重启或重载 OpenClaw 后生效。

## 四、使用示例

录入物品：

- "把牛奶录进冰箱，生产日期今天，保质期 7 天"
- "这盒鸡蛋 8 月 5 号过期，放冷藏室，记 12 个"
- "帮我记一下：客厅药箱里有一盒布洛芬"

查询：

- "看看这个月有什么快过期的东西"
- "冰箱里还有啥？"
- "查一下牛奶放在哪"

OpenClaw 会自动完成：解析位置名称 → 必要时创建位置 → 创建物品并推导到期日 → 用一句话向你确认。

## 五、故障排查

- **返回 401 / 提示未授权**
  - 检查 `HBOX_API_TOKEN` 是否完整复制（令牌只显示一次，丢字就会 401）。
  - 确认请求头格式是 `Authorization: Bearer <令牌>`，注意 `Bearer ` 前缀。
  - 令牌可能已被删除——回 Homebox 个人资料页重新创建一个。
- **连接失败 / 超时**
  - 先手动验证服务在线：`curl http://localhost:7745/api/v1/status`，正常会返回版本信息。
  - 检查 `HBOX_BASE_URL` 的协议、域名、端口（默认 7745）是否正确，远程部署需确认防火墙/反向代理放行了 `/api` 路径。
  - 用了 HTTPS 的话确认证书有效，自签证书可能导致 OpenClaw 请求被拒绝。
- **提示找不到位置**
  - 技能会自动创建缺失的位置；如果没有自动创建，先在 Homebox 网页 UI 里手动建好同名位置再试。
- **日期不对**
  - 所有日期都是 `YYYY-MM-DD` 格式；"保质期 N 天"由服务器自动换算成到期日，无需手动计算。
