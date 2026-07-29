---
name: homebox
description: 把物品录入 Homebox 家庭库存系统（支持生产日期/保质期/到期日），并查询临期物品。当用户说"把 X 录进/放进 Y"、"记录一下 X"、"查查快过期的东西"、"还有什么快到期"等意图时使用本技能。
---

# Homebox 库存助手

通过 Homebox REST API 帮用户管理家庭库存：录入物品（可带生产日期、保质期、到期日）、按位置存放、查询临期物品。

## 前置条件

需要两个环境变量（缺失时提示用户配置，不要猜测）：

- `HBOX_BASE_URL` — Homebox 服务地址，例如 `http://localhost:7745`（不要以 `/` 结尾）
- `HBOX_API_TOKEN` — 用户在 Homebox 网页 UI「个人资料 / Profile」页面创建的 API Token

所有请求都必须带请求头（注意 API Token 只能通过 Authorization 头传递，query 参数方式会被拒绝）：

```
Authorization: Bearer $HBOX_API_TOKEN
Content-Type: application/json
```

## API 要点（已与后端源码核对）

- 所有业务接口在 `/api/v1` 下；日期格式统一为 `YYYY-MM-DD`。
- **位置（Location）就是 entity**：没有独立的 `/locations` 接口。位置的 entity type 满足 `isLocation=true`。
- 创建物品 `POST /api/v1/entities`，请求体字段：
  - `name`（必填，1–255 字符）
  - `quantity`（数字）
  - `description`（可选，≤1000 字符）
  - `parentId`（可选，位置/父实体的 UUID —— 物品放进哪个位置就填那个位置的 ID）
  - `tagIds`（可选，UUID 数组）
  - `entityTypeId`（可选；不传时自动使用默认的"物品"类型）
  - `productionDate`（可选，`YYYY-MM-DD`）
  - `shelfLifeDays`（可选，整数）
  - `expiryDate`（可选，`YYYY-MM-DD`）
  - 到期日推导规则：`productionDate` + `shelfLifeDays`/`expiryDate` 任选其一给出时，服务器会自动算出另一个。
- 健康检查 `GET /api/v1/status`（无需鉴权），可用于排查连通性。

## 操作流程

### 0. 首次使用：自检

```bash
curl -s "$HBOX_BASE_URL/api/v1/status"
# 返回 {"version":...,"commit":...} 说明服务可达
curl -s -H "Authorization: Bearer $HBOX_API_TOKEN" "$HBOX_BASE_URL/api/v1/users/self"
# 返回 401 说明 token 无效，提示用户重新创建
```

### 1. 把位置名称解析成 ID

```bash
curl -s -H "Authorization: Bearer $HBOX_API_TOKEN" \
  "$HBOX_BASE_URL/api/v1/entities?isLocation=true&pageSize=200"
```

在返回的 `items` 数组里按 `name` 匹配（忽略大小写；用户说的"冰箱"可能存为"厨房冰箱"，优先精确匹配，其次包含匹配；有歧义时列出候选让用户选）。

如果位置不存在，先查出"位置类型"的 entityTypeId，再创建：

```bash
# 找到 isLocation=true 的类型，取其 id
curl -s -H "Authorization: Bearer $HBOX_API_TOKEN" "$HBOX_BASE_URL/api/v1/entity-types"

# 创建位置（把 <LOCATION_TYPE_ID> 换成上面查到的 id）
curl -s -X POST -H "Authorization: Bearer $HBOX_API_TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"冰箱","quantity":0,"description":"","tagIds":[],"entityTypeId":"<LOCATION_TYPE_ID>"}' \
  "$HBOX_BASE_URL/api/v1/entities"
```

创建物品前必须先确认位置存在；用户没指定位置时可省略 `parentId`。

### 2. （可选）把标签名称解析成 ID

```bash
curl -s -H "Authorization: Bearer $HBOX_API_TOKEN" "$HBOX_BASE_URL/api/v1/tags"
```

按 `name` 匹配取 `id` 放进 `tagIds`；标签不存在时跳过即可（也可以 `POST /api/v1/tags` 创建：`{"name":"...","description":""}`）。

### 3. 创建物品

```bash
curl -s -X POST -H "Authorization: Bearer $HBOX_API_TOKEN" -H "Content-Type: application/json" \
  -d '{
    "name": "牛奶",
    "quantity": 1,
    "description": "",
    "parentId": "<位置ID>",
    "tagIds": [],
    "productionDate": "2026-07-29",
    "shelfLifeDays": 7
  }' \
  "$HBOX_BASE_URL/api/v1/entities"
```

- 用户只说"生产日期今天、保质期 7 天"时，只传 `productionDate` 和 `shelfLifeDays`，服务器会自动算出 `expiryDate`。
- 用户直接说了到期日（"8 月 5 号过期"）就只传 `expiryDate`。
- "今天"要用执行时的实际日期换算成 `YYYY-MM-DD`。
- 成功后用一句话确认：物品名、数量、存放位置、到期日。

### 4. 查询临期物品

```bash
curl -s -H "Authorization: Bearer $HBOX_API_TOKEN" \
  "$HBOX_BASE_URL/api/v1/entities?expiringWithinDays=30&pageSize=200"
```

`expiringWithinDays=30` 表示"30 天内到期"（用户说"这周"就用 7，"这个月"用 30，未说明默认 30）。
若服务端版本尚不支持该参数（返回了全量列表），则退而求其次：在返回的 `items` 里按 `expiryDate` 字段自行过滤——`expiryDate` 非空且不晚于"今天+N 天"的即为临期。
把结果按到期日升序念给用户：名称、到期日、还剩几天、所在位置（`parent.name`）。已过期（到期日早于今天）的要单独提醒。

### 5. 搜索物品

```bash
curl -s -H "Authorization: Bearer $HBOX_API_TOKEN" \
  "$HBOX_BASE_URL/api/v1/entities?q=牛奶&pageSize=50"
```

## 完整示例："把牛奶录进冰箱，生产日期今天，保质期7天"

1. `GET /api/v1/entities?isLocation=true` → 找到 `name` 为"冰箱"的位置，记其 `id` 为 `LOC_ID`；找不到就按第 1 步创建。
2. 用实际日期组装请求（假设今天是 2026-07-29）：

```bash
curl -s -X POST -H "Authorization: Bearer $HBOX_API_TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"牛奶","quantity":1,"description":"","parentId":"LOC_ID","tagIds":[],"productionDate":"2026-07-29","shelfLifeDays":7}' \
  "$HBOX_BASE_URL/api/v1/entities"
```

3. 回复用户："已把牛奶（1 件）放进冰箱，生产日期 2026-07-29，保质期 7 天，到期日 2026-08-05。"

## 注意事项

- 401：token 错误、过期或没带 `Bearer ` 前缀 → 让用户去 Homebox 网页 UI 个人资料页重新创建 API Token。
- 连接失败：先 `GET /api/v1/status` 确认服务在线，再检查 `HBOX_BASE_URL` 是否带对了端口（默认 7745）、有没有多余的 `/api` 后缀。
- 创建返回 400 多为字段校验问题（名称超长、日期格式不对——必须 `YYYY-MM-DD`）。
- 不要替用户删除或修改已有物品，除非用户明确要求；本技能默认只做"录入"和"查询"。
