# telemetry-worker

Receives Lyrimuse's anonymous usage data ("Share Anonymous Usage Data" in Settings › About › Diagnostics & Data, on by default). It runs on Cloudflare Workers with a D1 database at `telemetry.yudaotor.me`.

| Endpoint | What |
|---|---|
| `POST /v1/heartbeat` | At most one per machine per day: random install ID, app and macOS version, chip, languages, which features are on, which built-in players played music and how many hangs on previous days |
| `POST /v1/crash` | One summary per crash report: exception, termination reason, top stack frames (paths already removed by the app) |
| `GET /v1/stats` | Aggregates; needs `Authorization: Bearer <ADMIN_TOKEN>` |
| `GET /` | Dashboard that reads `/v1/stats` with the token |

Only whitelisted fields are stored; strings and lists are truncated. No IP addresses are stored, only the country code Cloudflare derives from the request. The app never sends song titles, artists, local paths, account names or credentials (see `lyrimuse/Sources/LyrimuseCore/Telemetry/Telemetry.swift`).

## Deploy

```sh
npx wrangler d1 create lyrimuse-telemetry          # first time only; put the id into wrangler.toml
npx wrangler d1 execute lyrimuse-telemetry --remote --file schema.sql
npx wrangler deploy
npx wrangler secret put ADMIN_TOKEN
```

---

接收 Lyrimuse 的匿名使用数据（设置 › 关于 › 诊断与数据「发送匿名使用数据」，默认开），跑在 Cloudflare Workers + D1 上，域名 `telemetry.yudaotor.me`。只存白名单字段，不存 IP，只存 Cloudflare 按 IP 给的国家 / 地区码。App 不发歌名、歌手、本机路径、账号名和凭据。部署步骤同上。
