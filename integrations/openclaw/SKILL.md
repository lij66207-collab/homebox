---
name: homebox
description: 全面管理 Homebox 家庭库存系统：录入/批量录入物品（支持生产日期/保质期/到期日）、查询临期物品、移动位置、修改数量/标签/价格等信息、删除物品（含批量删除）。当用户说"把 X 录进/放进 Y"、"记录一下 X"、"批量录入"、"查查快过期的东西"、"把 X 移到 Y"、"把 X 改成/更新成"、"删掉/移除 X"、"把过期的都删了"等意图时使用本技能。
---

# Homebox 库存助手

通过 Homebox REST API 帮用户管理家庭库存：录入物品（可带生产日期、保质期、到期日）、按位置存放、查询临期物品，以及更新、移动、删除物品（含批量操作）。Web 界面上能做的物品管理操作，本技能都可以完成。

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
- 轻量修改 `PATCH /api/v1/entities/{id}`：只支持 `parentId`、`quantity`、`tagIds`、`entityTypeId` 四个字段，全部可选，传谁改谁。
- 完整更新 `PUT /api/v1/entities/{id}`：**全量语义**，除上述字段外还支持 `name`、`description`、`productionDate`/`shelfLifeDays`/`expiryDate`、`archived`、`insured`、`purchaseDate`/`purchaseFrom`/`purchasePrice`、`soldDate`/`soldTo`/`soldPrice`/`soldNotes`、`warrantyDetails`/`warrantyExpires`/`lifetimeWarranty`、`serialNumber`、`manufacturer`、`modelNumber`、`notes`、`assetId`、`fields`（自定义字段数组）等。改这些字段前必须先 `GET` 取完整对象，只改目标字段后整体回传。
- 删除 `DELETE /api/v1/entities/{id}`，成功返回 204，是硬删除。
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

`q` 支持名称模糊搜索；以 `#` 开头（如 `#000-123`）按资产编号精确查。

### 6. 把物品名称解析成 ID（更新/删除前的必经步骤）

更新或删除前，先用步骤 5 的搜索找到目标物品的 `id`：

- 只匹配到 1 个 → 直接使用其 `id`。
- 匹配到多个 → 列出候选（名称、位置、数量、到期日）让用户选，不要自行猜测。
- 用户明确指定了编号（`#xxx`）时优先用编号。

### 7. 轻量修改（移动位置 / 改数量 / 改标签）—— PATCH

只改位置、数量、标签、类型时用 `PATCH`，不需要先 GET：

```bash
# 例：把物品移到"储物间"，数量改为 3
curl -s -X PATCH -H "Authorization: Bearer $HBOX_API_TOKEN" -H "Content-Type: application/json" \
  -d '{"parentId":"<新位置ID>","quantity":3}' \
  "$HBOX_BASE_URL/api/v1/entities/<物品ID>"
```

- 只传要改的字段，其余不传。
- `tagIds` 是**整体替换**：要加减标签时，先 `GET /api/v1/entities/{id}` 读出现有 `tags`，合并/移除后把完整 `tagIds` 数组传回去。
- 注意 PATCH **不能**改名称、描述、日期、价格等字段，那些走步骤 8 的 PUT。

### 8. 完整更新（改名/描述/日期/价格/保修等）—— PUT

`PUT` 是全量更新：漏传的字段可能被清空。固定流程：

1. `GET /api/v1/entities/{id}` 拿到完整对象。
2. 只修改用户要求改的字段，其余字段（尤其 `fields` 自定义字段数组、`tagIds`）原样保留。
3. 整体 `PUT` 回传。

```bash
# 第 1 步：取全量
curl -s -H "Authorization: Bearer $HBOX_API_TOKEN" \
  "$HBOX_BASE_URL/api/v1/entities/<物品ID>"

# 第 3 步：在返回对象基础上改目标字段后整体回传（此处示例：改购买价格和购买日期）
curl -s -X PUT -H "Authorization: Bearer $HBOX_API_TOKEN" -H "Content-Type: application/json" \
  -d '<修改后的完整对象 JSON>' \
  "$HBOX_BASE_URL/api/v1/entities/<物品ID>"
```

### 9. 删除物品（含批量删除）

`DELETE /api/v1/entities/{id}`，成功返回 204，**是硬删除，不可恢复**。

强制流程（单个和批量都适用）：

1. 用步骤 6 解析出将受影响的物品清单。
2. **先把完整清单列给用户**（名称、位置、数量），明确获得用户确认后才执行。
3. 逐个调用：

```bash
curl -s -X DELETE -H "Authorization: Bearer $HBOX_API_TOKEN" \
  "$HBOX_BASE_URL/api/v1/entities/<物品ID>"
```

4. 批量删除时逐个执行并汇总结果："成功 N 条、失败 M 条（附失败原因）"。

其他约束：

- **禁止使用** `POST /api/v1/actions/wipe-inventory`（清空整个库存），任何情况下都不要调用。
- 删除有子实体的位置/容器前，先列出其子实体并询问用户如何处置（先移走或一并删除）。

### 10. 批量录入

用户一次报多件物品（"帮我把这几样都录进去：……"）时，两种方式：

**方式一：逐条创建（推荐，几十件以内）**

位置/标签只解析一次（步骤 1、2），然后循环步骤 3 逐条 `POST /api/v1/entities`。全部完成后汇总："成功 N 条、失败 M 条"，失败的附错误信息。

**方式二：CSV 导入（上百件的大批量）**

生成 CSV 后调用导入接口（multipart 表单，字段名固定为 `csv`）：

```bash
curl -s -X POST -H "Authorization: Bearer $HBOX_API_TOKEN" \
  -F "csv=@/tmp/homebox-import.csv" \
  "$HBOX_BASE_URL/api/v1/entities/import"
```

CSV 表头（常用列，均为可选，至少要有 `HB.name`）：

```
HB.import_ref,HB.name,HB.location,HB.description,HB.quantity,HB.purchase_price,HB.purchase_date,HB.tags
```

- `HB.location` 用 `/` 分隔层级，如 `厨房/冰箱`，导入时会自动创建缺失的位置。
- `HB.tags` 多个标签用 `;` 分隔，如 `食品; 冷藏`。
- `HB.import_ref` 是导入批次内的行标识；配合 `HB.parent_import_ref` 可以表达父子关系（容器装物品）。
- 日期 `YYYY-MM-DD`；价格为纯数字。
- 完整列清单可先 `GET /api/v1/entities/export` 下载导出 CSV 参考其表头（`HB.*` 前缀）。

## 完整示例一："把牛奶录进冰箱，生产日期今天，保质期7天"

1. `GET /api/v1/entities?isLocation=true` → 找到 `name` 为"冰箱"的位置，记其 `id` 为 `LOC_ID`；找不到就按第 1 步创建。
2. 用实际日期组装请求（假设今天是 2026-07-29）：

```bash
curl -s -X POST -H "Authorization: Bearer $HBOX_API_TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"牛奶","quantity":1,"description":"","parentId":"LOC_ID","tagIds":[],"productionDate":"2026-07-29","shelfLifeDays":7}' \
  "$HBOX_BASE_URL/api/v1/entities"
```

3. 回复用户："已把牛奶（1 件）放进冰箱，生产日期 2026-07-29，保质期 7 天，到期日 2026-08-05。"

## 完整示例二："把冰箱里过期的都删了"

1. `GET /api/v1/entities?isLocation=true` → 解析出"冰箱"的位置 ID `LOC_ID`。
2. `GET /api/v1/entities?parentIds=LOC_ID&pageSize=200` → 列出冰箱里的物品，按 `expiryDate` 早于今天筛出已过期的。
3. 把过期清单列给用户："冰箱里过期的有：牛奶（7-20 过期）、酸奶（7-25 过期）。确认删除吗？"
4. 用户确认后逐条 `DELETE /api/v1/entities/{id}`。
5. 回复："已删除 2 件过期物品：牛奶、酸奶。"

## 完整示例三："把牛奶移到储物间，数量改成 6"

1. 步骤 6 搜索"牛奶"得到物品 ID；步骤 1 解析"储物间"得到位置 ID。
2. 一条 PATCH 完成：

```bash
curl -s -X PATCH -H "Authorization: Bearer $HBOX_API_TOKEN" -H "Content-Type: application/json" \
  -d '{"parentId":"<储物间ID>","quantity":6}' \
  "$HBOX_BASE_URL/api/v1/entities/<牛奶ID>"
```

3. 回复："已把牛奶移到储物间，数量改为 6。"

## 注意事项

- 401：token 错误、过期或没带 `Bearer ` 前缀 → 让用户去 Homebox 网页 UI 个人资料页重新创建 API Token。
- 连接失败：先 `GET /api/v1/status` 确认服务在线，再检查 `HBOX_BASE_URL` 是否带对了端口（默认 7745）、有没有多余的 `/api` 后缀。
- 创建返回 400 多为字段校验问题（名称超长、日期格式不对——必须 `YYYY-MM-DD`）。
- **删除和修改是破坏性操作**：执行前必须先把将受影响的物品清单列给用户并获得明确确认；删除不可恢复。
- **禁止调用** `POST /api/v1/actions/wipe-inventory`（清空整个库存）。
- PUT 是全量语义：不先 GET 就 PUT 会清空未传字段；PATCH 只改位置/数量/标签/类型。
