# FindSenryu4Slack

Slackで川柳（5-7-5）を検出します。メッセージに5-7-5の音のまとまりを見つけると、そのスレッドに返信します。

[FindSenryu4Discord](../findsenryu4discord) のSlack移植版（検出MVP）。5-7-5判定のコアは
[go-haiku](https://github.com/0x307e/go-haiku)（[kagome](https://github.com/ikawaha/kagome) +
UniDic辞書）をそのまま利用しています。

## アーキテクチャ

- **言語:** Go（辞書はpure-Goでメモリ常駐 → 低レイテンシ）
- **接続:** Slack **Socket Mode**（アウトバウンドWebSocket。公開URL不要）
- **常駐プロセス:** 起動時にUniDic辞書を一度だけメモリへロードし、以降は温まった状態で判定するため応答が速い

## セットアップ

### 1. Slackアプリを作成

1. https://api.slack.com/apps → **Create New App** → **From a manifest**
2. ワークスペースを選び、[`manifest.yaml`](./manifest.yaml) を貼り付けて作成
3. **Install to Workspace** でインストールし、**Bot User OAuth Token**（`xoxb-…`）をコピー
4. **Basic Information → App-Level Tokens** で `connections:write` スコープのトークンを作成（`xapp-…`）
5. 検出したいチャンネルにBotを招待: `/invite @FindSenryu`

### 2. 環境変数

| 変数 | 必須 | 説明 |
| --- | --- | --- |
| `SLACK_BOT_TOKEN` | ✅ | Bot User OAuth Token（`xoxb-`） |
| `SLACK_APP_TOKEN` | ✅ | App-Level Token（`xapp-`, `connections:write`） |
| `LOG_LEVEL` | | `debug` / `info`（既定） / `warn` / `error` |
| `PORT` | | 設定時のみ `/health` エンドポイントを起動（App Runner等のヘルスチェック用） |

## ローカル実行

Socket Modeはアウトバウンド接続なので **ngrok等のトンネルは不要** です。

```sh
export SLACK_BOT_TOKEN=xoxb-...
export SLACK_APP_TOKEN=xapp-...
go run .
```

Botを招待したチャンネルで「古池や蛙飛び込む水の音」のような5-7-5を投稿すると、スレッドに
「川柳を検出しました！」と返信されます。

## テスト

```sh
go test -race ./...
```

## デプロイ

常駐プロセスとして稼働させます（辞書を温かく保つため）。

- **Render（Background Worker・おすすめ）:** このリポジトリ/Dockerfileから *Background Worker* を作成し、
  `SLACK_BOT_TOKEN` と `SLACK_APP_TOKEN` をSecretsに設定。公開ポート不要。
- **Fly.io:** `fly launch`（Dockerfileを検出）→ `fly secrets set SLACK_BOT_TOKEN=... SLACK_APP_TOKEN=...`。
  `fly.toml` で `min_machines_running = 1` にして常駐させる。
- **AWS App Runner:** `PORT` を設定して `/health` を有効化し、min instances = 1。

## スコープ

- **Bot Token:** `channels:history`, `chat:write`（必要に応じて `groups:history`, `channels:read`, `groups:read`）
- **App-Level Token:** `connections:write`
- **Event Subscriptions:** `message.channels`（必要に応じて `message.groups`）

