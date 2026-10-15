# 身份连接 · 类型图标素材

用途：控制台「身份连接」目录卡片 / 实例卡片 / 向导头部的类型图标
（见 `docs/specs/261013-identity-connections-phase3.md` FR-15、§5.6）。

## 收录清单

| 文件 | 图标 key | 来源 | 原始规格 | 备注 |
|---|---|---|---|---|
| `dingtalk.png` | `dingtalk` | 用户提供：`https://img.alicdn.com/imgextra/i1/O1CN01C0wRmQ1CSgTnrubJr_!!6000000000080-2-tps-278-276.png` | 278×276 PNG | 自带蓝色圆角底 |
| `wecom.png` | `wecom` | 用户提供：`https://img.alicdn.com/imgextra/i2/O1CN01fhZbak1CQOTmCagrT_!!6000000000075-2-tps-512-512.png` | 512×512 PNG | 透明底彩色气泡 |
| `wechat.png` | `wechat` | Casdoor 公开静态资源：`https://cdn.casbin.org/img/social_wechat.png` | 588×588 PNG | 透明底绿色气泡；Casdoor 登录页同款素材 |
| `feishu.png` | `feishu` | 用户提供：`https://img.alicdn.com/imgextra/i4/O1CN01QO2pzw1HqZPXCxyGc_!!6000000000809-2-tps-792-792.png` | 792×792 PNG | 透明底纸飞机 |
| `feishu.png`（复用） | `lark` | 同上 | — | 用户确认「Lark 与飞书用一样的」，映射表内指向 `feishu.png`，不重复存文件 |
| `ad.png` | `ad` | 用户提供：`https://img.alicdn.com/imgextra/i3/O1CN018K3D8V2A1XvXb4tDz_!!6000000008143-2-tps-200-200.png` | 200×200 PNG | 四窗格标识 |
| `openldap.png` | `ldap` | 用户提供：`https://img.alicdn.com/imgextra/i1/O1CN01W6hnia1pppUPJp1or_!!6000000005410-2-tps-254-254.png` | 254×254 PNG | 透明底红色标识 |
| `oidc.png` | `oidc` | 用户提供：`https://img.alicdn.com/imgextra/i2/O1CN012t6Drw1GQd6mU9kJt_!!6000000000617-2-tps-1000-1000.png` | 1000×1000 PNG | 透明底 OpenID 标识 |
| `mysql.svg` | `database`（`sub_type=mysql`） | 官方：`https://labs.mysql.com/common/logos/mysql-logo.svg` | 由 388×264 版式**裁切视口到海豚** | 仅改 `viewBox/width/height`，矢量路径与颜色保持原样 |
| `mysql-original.svg` | — | 同上（原版式） | 388×264 SVG | 留档：海豚 + 文字版式，供日后需要完整版式时使用 |
| `postgres.png` | `database`（`sub_type=postgres`） | 官方：`https://www.postgresql.org/media/img/about/press/elephant.png` | 540×557 PNG | 官方新闻素材大象 |

未使用：`https://www.mysql.com/cn/favicon.ico`（仅 16×16 栅格，放大到 24/40px 会糊，故弃用）。

## 兜底（代码内联矢量，无外部文件）

- `http`（通用 HTTP）：地球造型，多用于没有官方标识的自有/通用下游。
- `scim`（SCIM 2.0）：**本阶段不接入**（用户决定），目录不展示；若存在历史实例，列表用兜底图标以只读方式展示。
- 未知类型：插头/地球兜底，绝不出现空白占位。

## 约定

- 文件名与图标 key 一致（小写、无空格）；`lark` 是唯一的复用例外（映射表内指向 `feishu.png`）。
- 尺寸：目录/实例 32–40px；行内最小 24px；16px 只用于形状简单的图标（如钉钉）。
- 基底：统一中性底（白 / 暗色 `#2b2c43`）+ 1px 内描边 + 26% 圆角；官方素材按原比例 `object-contain`，不裁切、不叠加品牌色底。
- **商标与来源**：以上为各厂商品牌标识，来源于用户提供地址或厂商官方站点。产品对外发布前，请以各厂商官方品牌素材与使用规范为准。
