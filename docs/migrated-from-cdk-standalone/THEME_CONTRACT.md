# Theme Contract v1

Theme Contract 让代理商在不修改服务端和业务 JavaScript 的前提下，同时替换
公开兑换页与管理台的视觉主题。官方主题与第三方主题走同一加载、校验和回退
路径。

## 目录和清单

每个主题使用独立目录，目录名必须与 `theme.json.id` 一致：

```text
themes/merchant-theme/
  theme.json
  public.css
  admin.css
  assets/
    logo.webp
    display.woff2
```

`theme.json`：

```json
{
  "schema_version": 1,
  "id": "merchant-theme",
  "name": "Merchant Theme",
  "version": "1.0.0",
  "surfaces": ["public", "admin"],
  "styles": {
    "public": "public.css",
    "admin": "admin.css"
  },
  "assets": {
    "logo": "assets/logo.webp",
    "display_font": "assets/display.woff2"
  }
}
```

规则：

- `id` 只允许小写字母、数字和连字符，最长 48 字符。
- `version` 使用语义化版本。
- v1 主题必须同时实现 `public` 和 `admin` 才能完整替换两端。
- 样式文件最大 256 KiB，单个静态资源最大 2 MiB。
- 资源仅允许 PNG、JPEG、WebP、GIF、AVIF 和 WOFF2。
- v1 故意不接受 SVG、HTML 或 JavaScript，避免脚本、外链和注入风险。

## 稳定接口

根节点：

```css
[data-surface="public"] { /* 公开兑换页 */ }
[data-surface="admin"] { /* 管理台 */ }
```

稳定组件选择器使用 `data-component`，例如：

```css
[data-surface="public"] [data-component="panel"] {
  border-radius: 22px;
}

[data-surface="admin"] [data-component="theme-card"]::part(base) {
  border-color: var(--accent);
}
```

现有类名可以用于精细布局，但只有 `data-surface`、文档列出的
`data-component` 和 CSS tokens 属于兼容接口。主题不得根据按钮中文文案、
DOM 层级序号或自动生成的 Web Awesome 内部类名定位。

常用 tokens：

| Surface | Tokens |
| --- | --- |
| public | `--bg`、`--surface`、`--surface-2`、`--ink`、`--ink-2`、`--accent`、`--accent-2`、`--good`、`--warn`、`--err` |
| admin | `--bg`、`--panel`、`--panel-2`、`--line`、`--text`、`--muted`、`--accent`、`--accent-2`、`--danger`、`--warn`、`--radius` |

Web Awesome 组件通过标准标签和 CSS parts 定制。不要复制组件自身的键盘、
焦点、对话框或无障碍逻辑。

## 本地资源

CSS 只能引用当前主题 `assets/` 下的相对路径：

```css
@font-face {
  font-family: "Merchant Display";
  src: url("assets/display.woff2") format("woff2");
}
```

禁止：

- `@import`
- `http://`、`https://`、`//` 或任意远程 `url()`
- `data:`、`javascript:`、`expression()`、`behavior`、`-moz-binding`
- `..`、绝对路径和符号链接

只有在 `theme.json.assets` 中声明且通过类型校验的文件才会由
`/theme/assets/*` 提供。

## 安装和切换

1. 把主题目录放入宿主机 `themes/`。
2. 执行 `docker compose restart app`，让注册表重新扫描并校验新包。
3. 登录管理台，打开“站点主题”，点击“启用”。
4. 公开页与管理台刷新后立即使用新主题；不需要重建镜像。

无法解析、路径越界或包含远程 CSS 的主题会被忽略，并在主题页显示拒绝数量。
当前主题不存在或失效时自动回退 `core-default`。

可以用 `.env` 的 `THEME_ID` 指定首次启动默认主题；数据库中由管理员选择的
主题优先于环境默认值。
