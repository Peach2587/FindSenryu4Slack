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

`.env` にトークンを書いておくと、付属スクリプトが読み込んで起動します:

```sh
# .env
SLACK_BOT_TOKEN=xoxb-...
SLACK_APP_TOKEN=xapp-...
LOG_LEVEL=debug
```

- **`./local.sh`** — `.env` を読み込み `go run .` で起動（トークン必須チェック付き）
- **`./local_docker.sh`** — Dockerfileからイメージをビルドし、コンテナで起動（本番と同じ実行形態を確認できる）

素の`go run`でも動きます:

```sh
export SLACK_BOT_TOKEN=xoxb-...
export SLACK_APP_TOKEN=xapp-...
go run .
```

Botを招待したチャンネルで「古池や蛙飛び込む水の音」のような5-7-5を投稿すると、スレッドに
「川柳を検出しました！」と返信し、チャンネルにもブロードキャストされます。

> **Note:** DockerfileはGoのクロスコンパイル（`--platform=$BUILDPLATFORM`）を使うため、
> `local_docker.sh` でのビルドには **BuildKit（buildx）** が必要です。

## テスト

```sh
go test -race ./...
```

## デプロイ

常駐プロセスとして稼働させます（辞書を温かく保つため）。本番はコンテナイメージをKubernetes上で常駐運用しています。

### CI / イメージ

- `.github/workflows/build.yaml` が `main` へのpushで発火し、マルチアーキテクチャ（`linux/amd64,linux/arm64`）
  イメージをビルドして GHCR (`ghcr.io/urabeya/find-senryu-4-slack`) へpushします。
- DockerfileはGoのクロスコンパイル（`--platform=$BUILDPLATFORM` + `GOOS/GOARCH`）でQEMU不要のマルチarchビルドにしています。

### Kubernetes（GitOps / ArgoCD）

- マニフェストは別リポジトリ `urabeya/home-lab`（`manifests/find-senryu-4-slack/`）で管理し、ArgoCDが同期します。
- トークンはSecret `find-senryu-slack-tokens` から `envFrom` で注入。Socket Modeなので公開ポート不要。

### メモリ要件（重要）

起動時にUniDic辞書をメモリ常駐させるため、**定常で約420MiB** 消費します。コンテナのメモリ上限は余裕を持って
設定してください（本番: `requests 512Mi` / `limits 768Mi`）。GoのGCが上限を守るよう `GOMEMLIMIT`（例: `640MiB`）の
併用を推奨します。**上限が低すぎるとOOMKilled（exit 137）になります。**

### その他のホスティング

Render Background Worker / Fly.io / AWS App Runner などでも常駐プロセスとして動作します
（App Runnerは `PORT` を設定して `/health` を有効化し、min instances = 1）。

## バージョン管理

[セマンティックバージョニング](https://semver.org/lang/ja/) `x.y.z`（MAJOR.MINOR.PATCH）で管理します。
桁の上げ方は本リポジトリのコミット規約（Conventional Commits）と対応します:

| 桁 | 上げる条件 | 例 | 対応コミット |
| --- | --- | --- | --- |
| MAJOR (`x`) | 後方互換性のない変更 | `1.2.3` → `2.0.0` | `feat!:` / `BREAKING CHANGE` |
| MINOR (`y`) | 後方互換な機能追加 | `1.2.3` → `1.3.0` | `feat:` |
| PATCH (`z`) | 後方互換なバグ修正 | `1.2.3` → `1.2.4` | `fix:` |

`docs:` / `refactor:` / `test:` / `chore:` のみの変更ではバージョンを上げません。

### リリース手順

```sh
git tag -a v1.2.3 -m "release: v1.2.3"
git push origin v1.2.3
```

現状のCIは `main` へのpushで `:<commit-sha>` と `:latest` のイメージをGHCRへpushします。
バージョンタグ（`vX.Y.Z`）はリリース地点をGit上で固定するためのものです。
バージョン付きイメージ（`:1.2.3`）も配布したい場合は、`build.yaml` のトリガに `push: { tags: ['v*'] }` を追加し、
`tags:` に `${{ env.IMAGE }}:${{ github.ref_name }}` を加えます。

## スコープ

- **Bot Token:** `channels:history`, `chat:write`（必要に応じて `groups:history`, `channels:read`, `groups:read`）
- **App-Level Token:** `connections:write`
- **Event Subscriptions:** `message.channels`（必要に応じて `message.groups`）

