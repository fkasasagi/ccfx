[![English](https://img.shields.io/badge/README-English-lightgrey)](README.en.md)
[![日本語](https://img.shields.io/badge/README-%E6%97%A5%E6%9C%AC%E8%AA%9E-red)](README.md)

# ccfx - Claude Code Forensics eXtractor

Claude Code が残すローカルアーティファクト (`~/.claude/`) を解析し、**誰が・いつ・どのように** Claude Code を使ったかをレポートするデジタルフォレンジックツール。

Go 製シングルバイナリ。外部依存ゼロ。**Linux / macOS / Windows 対応。**

> **Windows について:** プロジェクトのパス (`C:\Users\...`・UNC パス) を Claude が記録した作業ディレクトリからそのまま復元し、`--timezone` の IANA 名 (`Asia/Tokyo` 等) にも対応します (タイムゾーンデータはバイナリに埋め込み済み)。`-ac` のアーカイブ取得のみ、シンボリックリンク保持のため管理者権限または開発者モードが必要です。

> **⚠ 注意:** 本ツールは検証段階です。出力内容の正確性を保証するものではありません。自己責任でご使用ください。
>
> 本ツールは正当な権限を持つユーザーによるフォレンジック分析・内部監査・セキュリティレビューを目的としています。他者のデータを無断で収集・解析する行為に使用しないでください。

---

## 分析対象アーティファクト

Claude Code は `~/.claude/` (Windows: `%USERPROFILE%\.claude\`) に以下のファイルを残します。ccfx はこれらを非破壊で読み取り、レポートを生成します。

| ファイル / ディレクトリ | 形式 | ccfx が抽出する情報 |
|---|---|---|
| `history.jsonl` | JSONL | ユーザーが入力したコマンド履歴、タイムスタンプ、セッション ID |
| `sessions/<pid>.json` | JSON | セッションメタデータ (PID, CWD, 開始/終了時刻, バージョン, entrypoint) |
| `projects/<encoded-path>/<uuid>.jsonl` | JSONL | **会話全文**、ツール呼び出し (名前・引数)、モデル、トークン使用量、Git ブランチ |
| `backups/.claude.json.backup.*` | JSON | ユーザー email、組織 UUID、サブスクリプション情報、プロジェクト別コスト |
| `settings.json` / `settings.local.json` | JSON | パーミッション (allow/deny ルール)、フック定義、effort level |
| `.credentials.json` | JSON | OAuth トークンの**存在検出のみ** (値は一切読みません) |
| `file-history/<session>/<hash>@v<n>` | 各種 | ファイル編集バージョン数の統計 |
| `shell-snapshots/`, `paste-cache/`, `tasks/`, `plans/` | 各種 | 補助メタデータのカウント |

## インストール

### ソースからビルド (Go 1.22+)

```bash
git clone https://github.com/fkasasagi/ccfx.git
cd ccfx
go build -o ccfx .
```

### クロスコンパイル

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o ccfx-linux .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o ccfx-darwin .

# Windows
GOOS=windows GOARCH=amd64 go build -o ccfx.exe .
```

## 使い方

```
ccfx [flags]
ccfx help [topic]
ccfx version
```

### フラグ一覧

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--path PATH` | 自動検出 (`~/.claude/`) | 解析対象ディレクトリのパス |
| `--format csv,json,md,html,all` | `all` | 出力フォーマット (カンマ区切りで複数指定可、`all` で全形式) |
| `--output DIR` | `./ccfx-output` | 出力先ディレクトリ |
| `--language en\|ja` | `en` | レポート言語 (英語 / 日本語) |
| `--extract-conversations` | on | 会話の全文をレポートに含める (`--extract-conversations=false` で無効化) |
| `--session-filter UUID` | - | 特定セッションのみ分析 |
| `--project-filter PATH` | - | 特定プロジェクトのみ分析 |
| `--date-from YYYY-MM-DD` | - | この日付以降に限定 |
| `--date-to YYYY-MM-DD` | - | この日付以前に限定 |
| `--timezone ZONE` | `UTC` | IANA タイムゾーン名 (例: `Asia/Tokyo`, `America/New_York`) |
| `--redact-pii` | off | email・UUID をマスク |
| `-ac` | off | `claude-acquisition.zip` も出力する。対象ディレクトリを丸ごと zip 化し、ファイルのタイムスタンプ・空ディレクトリ・シンボリックリンクを維持する。**`.credentials.json` を平文のまま含む — [セキュリティに関する注意](#セキュリティに関する注意)を参照** |
| `--force` | off | 出力先に既存ファイルがあっても上書きする (未指定時は中止) |
| `--verbose` | off | デバッグログ出力 |
| `--version` | - | バージョン表示 |
| `--help` | - | ヘルプ表示 |

### ヘルプシステム

`ccfx help [topic]` でトピック別の詳細ヘルプを表示できます。

| コマンド | 内容 |
|---|---|
| `ccfx help` | メインヘルプ (フラグ一覧、使用例、トピック一覧) |
| `ccfx help artifacts` | 解析対象ファイルの場所・形式・抽出情報 |
| `ccfx help formats` | CSV / JSON / Markdown / HTML 各フォーマットの詳細 |
| `ccfx help report` | レポート 14 セクションの内容説明 |
| `ccfx help injection` | セッションがプロンプトインジェクションを受けたかの判断方法: 何を見るか、シグナルの分類、想定される偽陽性 |
| `ccfx help security` | 認証情報の扱い、PII マスク、read-only 動作 |
| `ccfx help timezone` | 使用可能な IANA タイムゾーン名の一覧 |
| `ccfx help examples` | IR・内部監査・セキュリティレビュー等のワークフロー例 |

### 基本的な使い方

```bash
# JSON レポートのみ (デフォルト)
./ccfx

# 全フォーマットを一括出力
./ccfx --format all

# 全フォーマットを日本語で出力
./ccfx --format all --language ja

# 別マシンの Claude データを解析 (USB マウント等)
./ccfx --path /mnt/evidence/home/user/.claude --format json,html

# 特定期間に絞って PII マスク付きでレポート
./ccfx --format html --date-from 2026-05-01 --date-to 2026-05-31 --redact-pii

# タイムスタンプを日本時間 (JST) で出力
./ccfx --format all --timezone Asia/Tokyo --language ja

# プロンプトインジェクションの疑いを調べる (レポートのセクション 14)
./ccfx --format html --language ja
# → report.html のセクション 14 で、検出事項 → 要確認セッション → 該当箇所の抜粋の順に読む

# 特定セッションの会話全文を抽出
./ccfx --format json --session-filter "a1b2c3d4-e5f6-7890-abcd-ef1234567890" --extract-conversations
```

`--timezone` を指定すると、全出力のタイムスタンプが変換され、カラム名にタイムゾーン略称が付きます (例: `Started At (JST)`)。

## 出力ファイル

`--format all` (または `--format csv,json,md,html`) で全フォーマットを指定した場合:

```
ccfx-output/
├── report.json          # 完全なレポート (構造化 JSON)
├── report.md            # Markdown レポート (14 セクション)
├── report.html          # 自己完結型 HTML (CSS 埋め込み、ダークテーマ)
├── sessions.csv         # セッション一覧
├── timeline.csv         # アクティビティタイムライン
├── tool_usage.csv       # ツール使用統計
├── file_changes.csv     # ファイル変更記録
├── token_usage.csv      # 日別トークン消費量
├── history.csv          # コマンド入力履歴
├── conversations.csv    # 会話メッセージ全文 (1 メッセージ 1 行)
├── injection_events.csv     # 入出力の全件一覧
└── injection_findings.csv   # 相関を取った検出事項 (重大度順)
```

CSV には UTF-8 BOM が付いているため、Windows Excel で文字化けせずに開けます。

`-ac` を付けると、上記に加えてもう1ファイル出力されます。

```
└── claude-acquisition.zip   # 対象ディレクトリの完全な複製 (タイムスタンプ維持)
```

## 出力例

### JSON (抜粋)

```json
{
  "meta": {
    "generated_at": "2026-05-25T13:38:19Z",
    "source_path": "/home/user/.claude",
    "tool_version": "0.7.2",
    "platform": "linux/amd64",
    "total_sessions": 310,
    "total_projects": 6,
    "date_range": {
      "earliest": "2026-05-12T13:16:00Z",
      "latest": "2026-05-25T13:38:12Z"
    }
  },
  "user_identity": {
    "email": "user@example.com",
    "account_uuid": "a1b2c3d4-****-****-****-************",
    "organization_type": "claude_max",
    "organization_role": "admin",
    "rate_limit_tier": "default_claude_max_5x",
    "claude_code_version": "2.1.139"
  },
  "tool_usage": {
    "top_tools": [
      { "tool_name": "Bash",       "total_calls": 440, "session_count": 9 },
      { "tool_name": "Edit",       "total_calls": 149, "session_count": 6 },
      { "tool_name": "TaskUpdate", "total_calls": 144, "session_count": 4 },
      { "tool_name": "Write",      "total_calls": 139, "session_count": 7 },
      { "tool_name": "TaskCreate", "total_calls":  73, "session_count": 4 }
    ]
  },
  "token_usage": {
    "total_input": 115342,
    "total_output": 2383171,
    "total_cache_creation": 8404208,
    "total_cache_read": 262189086
  },
  "credentials": {
    "file_exists": true,
    "file_modified_at": "2026-05-25T20:54:23Z",
    "file_size_bytes": 470,
    "oauth_token_detected": true
  },
  "command_history": [
    { "display": "/plan", "timestamp": "2026-05-12T13:16:00Z", "project": "/home/user/myproject", "sessionId": "a1b2c3d4-..." }
  ],
  "file_history_stats": {
    "session_count": 12,
    "total_file_versions": 270
  },
  "misc_stats": {
    "shell_snapshots": 3,
    "paste_cache_files": 1,
    "task_sessions": 4,
    "plan_files": 4,
    "custom_commands": 1
  }
}
```

### CSV - tool_usage.csv

```
Tool Name,Total Calls,Session Count
Bash,440,9
Edit,149,6
TaskUpdate,144,4
Write,139,7
TaskCreate,73,4
WebSearch,59,8
Read,43,9
```

### CSV - token_usage.csv

```
Date,Input Tokens,Output Tokens,Cache Creation,Cache Read
2026-05-12,6877,598573,1322042,26582115
2026-05-13,308,182360,621051,10105482
2026-05-15,94002,667665,2808707,27135615
2026-05-16,4737,619171,1650754,167553266
2026-05-20,178,81111,1043877,16517713
```

### CSV - history.csv

```
Timestamp (UTC),Session ID,Project,Shell?,Command
2026-05-12 13:16:00,11112222-3333-4444-5555-666677778888,/home/user/myproject,false,/plan
2026-05-12 13:17:07,11112222-3333-4444-5555-666677778888,/home/user/myproject,false,/effort
2026-05-12 13:18:32,11112222-3333-4444-5555-666677778888,/home/user/myproject,false,fix the login bug
2026-05-13 09:42:15,c4d5e6f7-a8b9-0123-cdef-456789abcdef,/home/user/api,true,!git status
```

`Shell?` 列は、プロンプトの `!` バンモードでサブシェル実行されたエントリのとき `true`、通常プロンプトやスラッシュコマンドのとき `false` になります。

### Markdown (冒頭)

```markdown
# Claude Code Forensic Analysis Report

- **Generated At**: 2026-05-25T13:38:19Z
- **Total Sessions**: 310
- **Total Projects**: 6
- **Date Range**: 2026-05-12 ~ 2026-05-25

## 1. User Identity

- **Email**: user@example.com
- **Organization Type**: claude_max
- **Rate Limit Tier**: default_claude_max_5x

## 2. Sessions
| Session ID | Project | Started At | Duration (min) | Model | Messages |
|---|---|---|---|---|---|
| `a1b2c3d4` | /home/user/myproject | 2026-05-12 13:16:00 | 47.2 | claude-opus-4-7 | 139 |
```

### HTML

自己完結型の HTML ファイル (CSS・JS 埋め込み、外部リソース不要) が生成されます。ダークテーマ、折りたたみ可能テーブル、CSS バーチャートに加え、大きなテーブル (Sessions / Timeline / File Changes / Command History / Conversations) には**各カラムのフィルター入力欄**が付き、その場で行を絞り込めます。ブラウザで直接開け、印刷にも対応 (`@media print`。印刷時はフィルターを非表示)。

## レポートセクション

生成されるレポートには以下のセクションが含まれます:

| # | セクション | 内容 |
|---|---|---|
| 1 | User Identity | ユーザー email、組織情報、サブスクリプションティア |
| 2 | Sessions | 全セッション一覧 (開始時刻、所要時間、モデル、メッセージ数) |
| 3 | Activity Timeline | 時系列のユーザー操作・ツール呼び出し・応答 |
| 4 | Projects | プロジェクト別サマリー (セッション数、初回/最終使用) |
| 5 | Tool Usage Statistics | ツール別の呼び出し回数・使用セッション数 |
| 6 | Token Consumption | トークン消費量 (モデル別、プロジェクト別、日付別) |
| 7 | File Modifications | Edit/Write ツールによるファイル変更記録 |
| 8 | Permission & Security | deny/allow ルール、フック定義、セッション別パーミッションモード |
| 9 | Credential Discovery | `.credentials.json` の存在検出 (トークン値は読みません) |
| 10 | Command History | `history.jsonl` のコマンド入力履歴 (タイムスタンプ、セッション、プロジェクト、`!` バンモードのサブシェル実行を示す `Shell?` フラグ) |
| 11 | File History Statistics | `file-history/` のファイル編集セッション数・バージョン総数 |
| 12 | Auxiliary Artifact Statistics | shell-snapshots, paste-cache, tasks, plans, custom-commands の件数 |
| 13 | Conversations | 会話全文 (デフォルトで含む。`--extract-conversations=false` で除外) |
| 14 | Prompt Injection Triage | 各セッションに何が入り (取得した URL・読んだファイル・hook が注入したテキスト)、そのテキストがどう見え、その後に何が出て、何が変更されたか。相関を取って重大度順に提示する。`ccfx help injection` を参照 |

## セキュリティに関する注意

- **認証トークンの値は一切読み取りません。** `.credentials.json` についてはファイルの存在、サイズ、更新日時のみを記録します。
- **`-ac` だけは例外です。** 取得アーカイブは対象ディレクトリのバイト単位の複製なので、`.credentials.json` を平文のまま含み、`--redact-pii` も適用されません。アーカイブを手にした者は取得元ユーザーとして認証できてしまいます。受け渡し時・保管時とも暗号化し、レポートだけで用が足りるなら `-ac` は付けないでください。
- `--redact-pii` を使うと、出力レポート中の email アドレスと UUID がマスクされます (`us***@example.com`, `a1b2c3d4-****-****-****-************`)。
- 入力ディレクトリ (`~/.claude/`) の内容は一切変更しません (read-only 解析)。

## プロジェクト構成

```
ccfx/
├── main.go              # CLI エントリポイント
├── model/model.go       # データモデル定義
├── collector/           # アーティファクト収集 (8 パーサー)
│   ├── collector.go     #   オーケストレーター
│   ├── history.go       #   history.jsonl
│   ├── sessions.go      #   sessions/<pid>.json
│   ├── transcripts.go   #   projects/<path>/<uuid>.jsonl
│   ├── backups.go       #   backups/.claude.json.backup.*
│   ├── settings.go      #   settings.json
│   ├── credentials.go   #   .credentials.json (存在検出のみ)
│   ├── filehistory.go   #   file-history/
│   └── misc.go          #   shell-snapshots, paste-cache 等
├── analyzer/            # フォレンジック分析
│   ├── analyzer.go      #   RawData → ForensicReport 変換
│   ├── timeline.go      #   時系列イベント構築
│   ├── toolusage.go     #   ツール使用統計
│   ├── tokenusage.go    #   トークン消費分析
│   ├── filetracking.go  #   ファイル変更追跡
│   └── permissions.go   #   パーミッション分析
└── renderer/            # レポート出力
    ├── renderer.go      #   フォーマット振り分け
    ├── json.go          #   JSON 出力
    ├── csv.go           #   CSV 出力 (UTF-8 BOM 付き)
    ├── markdown.go      #   Markdown 出力
    ├── html.go          #   HTML 出力 (自己完結型)
    └── locale.go        #   日英バイリンガル辞書
```

## フォレンジック活用例

### インシデント対応
```bash
# 退職者の PC から Claude Code の使用状況を調査 (現地時間で表示)
./ccfx --path /mnt/evidence/Users/suspect/.claude \
       --format json,html \
       --timezone Asia/Tokyo \
       --extract-conversations \
       --redact-pii
```

### 内部監査
```bash
# 今月の使用状況を CSV で取得し、スプレッドシートに取り込む
./ccfx --format csv \
       --timezone Asia/Tokyo \
       --date-from 2026-05-01 --date-to 2026-05-31 \
       --language ja
```

### セキュリティレビュー
```bash
# パーミッション設定と認証情報の状態を確認
./ccfx --format html --language ja
# → report.html の "パーミッション・セキュリティ設定" と "認証情報検出" を確認
```

## 免責事項

- 本ツールは検証段階 (experimental) です。出力結果の正確性・完全性を保証しません。
- 使用により生じた損害について、開発者は一切の責任を負いません。
- 本ツールは正当な権限に基づく使用を前提としています。無断でのデータ収集・解析への悪用を禁じます。

## ライセンス

MIT
